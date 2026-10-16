package oidc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type argoSettings struct {
	OIDCConfig *oidcConfig `json:"oidcConfig"`
}

type oidcConfig struct {
	Issuer   string   `json:"issuer"`
	ClientID string   `json:"clientID"`
	Scopes   []string `json:"scopes"`
}

// Login performs OIDC Authorization Code + PKCE against ArgoCD/Dex.
// It fetches the OIDC config from the ArgoCD server, opens a browser for
// the user to authenticate, and returns the id_token for use as --auth-token.
// The returned token is the user's own OIDC identity — not a shared service account.
func Login(serverURL string, tlsSkipVerify bool) (string, error) {
	client := httpClient(tlsSkipVerify)

	cfg, err := fetchOIDCConfig(client, serverURL)
	if err != nil {
		return "", err
	}

	issuer := strings.TrimRight(cfg.Issuer, "/")
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email", "groups"}
	}

	// Discover the real authorization and token endpoints from the OIDC provider.
	// Keycloak's endpoints are at /protocol/openid-connect/{auth,token}, not /auth.
	endpoints, err := discoverEndpoints(client, issuer)
	if err != nil {
		return "", err
	}

	verifier, challenge, err := pkce()
	if err != nil {
		return "", fmt.Errorf("generate PKCE: %w", err)
	}

	state, err := randomString(16)
	if err != nil {
		return "", fmt.Errorf("generate state: %w", err)
	}

	// Fixed port so Keycloak/Dex can whitelist an exact redirect URI.
	// Register http://127.0.0.1:7075/auth/callback in your OIDC client.
	const callbackPort = 7075
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return "", fmt.Errorf(
			"cannot bind callback port 7075: %w\n"+
				"  → Another process may be using port 7075 — stop it and retry",
			err)
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/auth/callback", callbackPort)

	authURL := endpoints.AuthorizationEndpoint + "?" + url.Values{
		"client_id":             {cfg.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}

	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			errCh <- fmt.Errorf("OIDC state mismatch — possible CSRF, retry")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := q.Get("error"); e != "" {
			errCh <- fmt.Errorf("OIDC error: %s — %s", e, q.Get("error_description"))
			fmt.Fprintf(w, "<html><body><h2>Auth failed: %s</h2><p>You can close this tab.</p></body></html>", e)
			return
		}
		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("OIDC callback missing code parameter")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, "<html><body><h2>✅ Authenticated — you can close this tab and return to the terminal.</h2></body></html>")
		codeCh <- code
	})

	go func() { _ = srv.Serve(listener) }()

	fmt.Printf("\n🌐 Opening browser for ArgoCD OIDC login...\n")
	fmt.Printf("   If the browser does not open automatically, visit:\n   %s\n\n", authURL)
	openBrowser(authURL)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var code string
	select {
	case code = <-codeCh:
	case err = <-errCh:
		_ = srv.Shutdown(context.Background())
		return "", err
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return "", fmt.Errorf("timed out waiting for OIDC callback (5 min) — check your browser")
	}
	_ = srv.Shutdown(context.Background())

	fmt.Println("✅ Browser authentication complete.")

	token, err := exchangeCode(client, endpoints.TokenEndpoint, cfg.ClientID, code, verifier, redirectURI)
	if err != nil {
		return "", err
	}
	printTokenClaims(token)
	return token, nil
}

func fetchOIDCConfig(client *http.Client, serverURL string) (*oidcConfig, error) {
	resp, err := client.Get(strings.TrimRight(serverURL, "/") + "/api/v1/settings")
	if err != nil {
		return nil, fmt.Errorf("cannot reach ArgoCD at %s: %w", serverURL, err)
	}
	defer resp.Body.Close()

	var s argoSettings
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("decode ArgoCD settings: %w", err)
	}
	if s.OIDCConfig == nil || s.OIDCConfig.Issuer == "" {
		return nil, fmt.Errorf(
			"ArgoCD at %s has no OIDC configured\n"+
				"  → Set up Dex or an external OIDC provider in your ArgoCD argocd-cm configmap",
			serverURL)
	}
	return s.OIDCConfig, nil
}

func exchangeCode(client *http.Client, tokenURL, clientID, code, verifier, redirectURI string) (string, error) {
	resp, err := client.PostForm(tokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	})
	if err != nil {
		return "", fmt.Errorf("token exchange request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tr struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
		Desc    string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.Error != "" {
		return "", fmt.Errorf("token exchange failed: %s — %s", tr.Error, tr.Desc)
	}
	if tr.IDToken == "" {
		return "", fmt.Errorf("OIDC provider returned no id_token")
	}
	return tr.IDToken, nil
}

type oidcEndpoints struct {
	AuthorizationEndpoint string
	TokenEndpoint         string
}

func discoverEndpoints(client *http.Client, issuer string) (*oidcEndpoints, error) {
	resp, err := client.Get(issuer + "/.well-known/openid-configuration")
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery at %s: %w", issuer, err)
	}
	defer resp.Body.Close()

	var doc struct {
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode OIDC discovery document: %w", err)
	}
	if doc.AuthorizationEndpoint == "" {
		return nil, fmt.Errorf("authorization_endpoint missing in OIDC discovery at %s", issuer)
	}
	if doc.TokenEndpoint == "" {
		return nil, fmt.Errorf("token_endpoint missing in OIDC discovery at %s", issuer)
	}
	return &oidcEndpoints{
		AuthorizationEndpoint: doc.AuthorizationEndpoint,
		TokenEndpoint:         doc.TokenEndpoint,
	}, nil
}

// printTokenClaims decodes the JWT payload and prints the identity-relevant
// claims so the user can verify group names match the ArgoCD RBAC policy.
func printTokenClaims(token string) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return
	}
	fmt.Println("\n🪪 Token identity:")
	for _, key := range []string{"sub", "preferred_username", "email", "groups"} {
		if v, ok := claims[key]; ok {
			fmt.Printf("   %-20s %v\n", key, v)
		}
	}
	fmt.Println()
}

// pkce generates a PKCE verifier and its S256 challenge.
func pkce() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(u string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{u}
	case "windows":
		cmd, args = "rundll32", []string{"url.dll,FileProtocolHandler", u}
	default:
		cmd, args = "xdg-open", []string{u}
	}
	_ = exec.Command(cmd, args...).Start()
}

func httpClient(skipVerify bool) *http.Client {
	if !skipVerify {
		return http.DefaultClient
	}
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // intentional for dev environments
		},
	}
}
