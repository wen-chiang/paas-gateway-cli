package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/paas/pgate/internal/config"
	"golang.org/x/term"
)

const (
	envUser  = "PGATE_GITHUB_USER"
	envToken = "PGATE_GITHUB_TOKEN"
)

// Identity holds the authenticated GitHub user.
type Identity struct {
	Login    string
	Email    string
	Token    string // password — used for Basic Auth in downstream API calls, never printed
	AuthType string // "basic"
}

type githubUser struct {
	Login string `json:"login"`
	Email string `json:"email"`
}

// GitHubLogin authenticates against GitHub.
// Credentials are read from PGATE_GITHUB_USER / PGATE_GITHUB_TOKEN env vars when set,
// allowing non-interactive use from Claude Code or CI. Falls back to interactive prompts.
func GitHubLogin() (*Identity, error) {
	fmt.Printf("\n🔐 GitHub login (%s)\n", config.Global.GitHub.WebURL())

	username, password := os.Getenv(envUser), os.Getenv(envToken)

	if username != "" && password != "" {
		fmt.Printf("   Using credentials from %s / %s\n", envUser, envToken)
	} else {
		if username == "" {
			fmt.Print("   Username: ")
			if _, err := fmt.Scanln(&username); err != nil {
				return nil, fmt.Errorf("reading username: %w", err)
			}
			username = strings.TrimSpace(username)
		}

		fmt.Print("   Password: ")
		passwdBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		if err != nil {
			return nil, fmt.Errorf("reading password: %w", err)
		}
		password = strings.TrimSpace(string(passwdBytes))
	}

	user, err := verifyCredentials(username, password)
	if err != nil {
		return nil, err
	}

	return &Identity{
		Login:    user.Login,
		Email:    user.Email,
		Token:    password,
		AuthType: "basic",
	}, nil
}

func verifyCredentials(username, password string) (*githubUser, error) {
	req, err := http.NewRequest(http.MethodGet, config.Global.GitHub.APIURL()+"/user", nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", config.Global.GitHub.APIURL(), err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// success — fall through
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("invalid username or password")
	case http.StatusForbidden:
		return nil, fmt.Errorf("access forbidden — check your account permissions")
	default:
		return nil, fmt.Errorf("GitHub API returned HTTP %d", resp.StatusCode)
	}

	var u githubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("decode user response: %w", err)
	}
	if u.Login == "" {
		return nil, fmt.Errorf("GitHub returned empty user — check credentials")
	}
	return &u, nil
}
