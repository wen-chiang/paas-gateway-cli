package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/paas/pgate/internal/auth"
	"github.com/paas/pgate/internal/config"
	oidcpkg "github.com/paas/pgate/internal/oidc"
	"github.com/paas/pgate/internal/policy"
	"github.com/spf13/cobra"
)

var (
	repo   string
	env    string
	branch string
	crq    string
	dryRun bool
)

var appNameVerbs = map[string]bool{
	"sync": true, "get": true, "wait": true, "delete": true,
	"history": true, "rollback": true, "set": true, "unset": true,
	"patch": true, "terminate-op": true, "logs": true,
}

var argocdCmd = &cobra.Command{
	Use:   "argocd",
	Short: "ArgoCD commands with policy enforcement",
}

var argocdLoginCmd = &cobra.Command{
	Use:     "login",
	Short:   "Authenticate via GitHub and get a scoped ArgoCD deploy token",
	Example: `  pgate argocd login --repo myorg/payment-service --env staging`,
	RunE:    runArgocdLogin,
}

var argocdRunCmd = &cobra.Command{
	Use:     "run",
	Short:   "Run any ArgoCD command with policy enforcement — token never exposed",
	Example: `  pgate argocd run --repo myorg/payment-service --env dev -- app sync payment-service-dev`,
	Args:    cobra.ArbitraryArgs,
	RunE:    runArgocdProxy,
}

func init() {
	pf := argocdCmd.PersistentFlags()
	pf.StringVar(&repo, "repo", "", "GitHub repo (org/name), e.g. myorg/payment-service")
	pf.StringVar(&env, "env", "", "Target environment, e.g. dev, trial, prod")
	pf.StringVar(&branch, "branch", "", "Branch to read .pgate.yaml and CODEOWNERS from (default: repo default branch)")
	pf.StringVar(&crq, "crq", "", "Change request number, required for prod (e.g. CHG0012345)")
	pf.BoolVar(&dryRun, "dry-run", false, "Run policy checks only — do not execute or issue a token")
	_ = argocdCmd.MarkPersistentFlagRequired("repo")
	_ = argocdCmd.MarkPersistentFlagRequired("env")

	argocdCmd.AddCommand(argocdLoginCmd)
	argocdCmd.AddCommand(argocdRunCmd)
	rootCmd.AddCommand(argocdCmd)
}

// ── shared helpers ────────────────────────────────────────────────────────────

func gateAndToken(repoFlag, envFlag, branchFlag, crFlag string) (token, appName string, toolCfg config.ToolConfig, err error) {
	fmt.Println()
	identity, err := auth.GitHubLogin()
	if err != nil {
		return "", "", config.ToolConfig{}, fmt.Errorf("authentication failed: %w", err)
	}
	fmt.Printf("✅ Authenticated as: %s\n", identity.Login)

	repoCfg := config.LoadRepoConfig(repoFlag, branchFlag, identity.Login, identity.Token)

	fmt.Println("\n📋 Policy checks...")
	failures := policy.RunGate(policy.Input{
		Repo:     repoFlag,
		Env:      envFlag,
		Branch:   branchFlag,
		CR:       crFlag,
		Identity: identity,
		RepoCfg:  repoCfg,
	})
	if len(failures) > 0 {
		return "", "", config.ToolConfig{}, fmt.Errorf("🚫 token denied — failed checks: %s", strings.Join(failures, ", "))
	}

	appName = config.ResolveAppName(repoFlag, envFlag, repoCfg)

	toolCfg, err = config.Global.ToolFor(envFlag, "argocd")
	if err != nil {
		return "", "", config.ToolConfig{}, err
	}

	if dryRun {
		return "", appName, toolCfg, nil
	}

	token, err = oidcpkg.Login(toolCfg.ServerURL, toolCfg.TLSSkipVerify)
	if err != nil {
		return "", "", config.ToolConfig{}, fmt.Errorf("ArgoCD OIDC login failed: %w", err)
	}
	return token, appName, toolCfg, nil
}

// extractAppName returns the app name targeted by argocd passthrough args.
// Returns "" for commands that don't target a specific app (e.g. app list).
//
//	["app", "sync", "myapp"]        → "myapp"
//	["app", "--flag", "sync", "myapp"] → "myapp"
//	["app", "list"]                 → ""
func extractAppName(args []string) string {
	for i, a := range args {
		if a == "app" && i+1 < len(args) {
			verb := ""
			verbIdx := -1
			for j := i + 1; j < len(args); j++ {
				if !strings.HasPrefix(args[j], "-") {
					verb = args[j]
					verbIdx = j
					break
				}
			}
			if !appNameVerbs[verb] {
				return ""
			}
			for _, candidate := range args[verbIdx+1:] {
				if !strings.HasPrefix(candidate, "-") {
					return candidate
				}
			}
		}
	}
	return ""
}

func maskToken(t string) string {
	if len(t) <= 16 {
		return "***"
	}
	return t[:16] + "..."
}

func serverHost(rawURL string) string {
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	return strings.TrimRight(rawURL, "/")
}

// ── command handlers ──────────────────────────────────────────────────────────

func runArgocdLogin(_ *cobra.Command, _ []string) error {
	token, appName, toolCfg, err := gateAndToken(repo, env, branch, crq)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("\n✅ Dry run — policy passed.\n")
		fmt.Printf("   ArgoCD app : %s\n", appName)
		fmt.Printf("   Server     : %s\n", toolCfg.ServerURL)
		fmt.Printf("\n   Next step  : pgate argocd run --repo %s --env %s -- app sync %s\n", repo, env, appName)
		fmt.Println("\n   (dry-run: no token issued, no deployment triggered)")
		return nil
	}

	fmt.Printf("\n🎫 Token issued (10 min)\n")
	fmt.Printf("   Token : %s\n", maskToken(token))
	fmt.Printf("\n   Next  : pgate argocd run --repo %s --env %s -- app sync %s\n\n", repo, env, appName)
	return nil
}

func runArgocdProxy(_ *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no argocd command given\n  example: pgate argocd run --repo x --env dev -- app sync <appname>")
	}

	token, appName, toolCfg, err := gateAndToken(repo, env, branch, crq)
	if err != nil {
		return err
	}

	if cmdApp := extractAppName(args); cmdApp != "" && cmdApp != appName {
		return fmt.Errorf(
			"🚫 app name mismatch\n"+
				"   command targets : %s\n"+
				"   gated for       : %s (repo=%s env=%s)",
			cmdApp, appName, repo, env)
	}

	if dryRun {
		fmt.Printf("\n✅ Dry run — policy passed, app name validated.\n")
		fmt.Printf("   Would run : argocd %s\n", strings.Join(args, " "))
		fmt.Printf("   Server    : %s\n\n", toolCfg.ServerURL)
		fmt.Println("   (dry-run: command not executed)")
		return nil
	}

	argocdBin, err := exec.LookPath("argocd")
	if err != nil {
		return fmt.Errorf("argocd CLI not found in PATH\n  install: https://argo-cd.readthedocs.io/en/stable/cli_installation/")
	}

	argocdArgs := make([]string, len(args), len(args)+4)
	copy(argocdArgs, args)
	argocdArgs = append(argocdArgs,
		"--server", serverHost(toolCfg.ServerURL),
		"--auth-token", token,
	)
	if toolCfg.TLSSkipVerify {
		argocdArgs = append(argocdArgs, "--insecure")
	}
	if toolCfg.GRPCWeb {
		argocdArgs = append(argocdArgs, "--grpc-web")
	}

	fmt.Printf("\n▶  argocd %s\n\n", strings.Join(args, " "))

	// Temp HOME prevents argocd from loading stale cached sessions.
	tmpHome, err := os.MkdirTemp("", "pgate-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpHome)

	cmd := exec.Command(argocdBin, argocdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "HOME="+tmpHome)
	return cmd.Run()
}
