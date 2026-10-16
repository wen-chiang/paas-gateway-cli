package policy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/paas/pgate/internal/auth"
	"github.com/paas/pgate/internal/config"
)

// Input carries all context needed by every policy check.
type Input struct {
	Repo     string
	Env      string
	Branch   string // empty = repo default branch
	CR       string // change request number, required for prod
	Identity *auth.Identity
	RepoCfg  *config.RepoConfig
}

type check struct {
	key  string // matches config.yaml policy.checks key
	name string
	run  func() (detail string, ok bool)
}

// RunGate runs policy checks in order.
//
// Behaviour on failure is controlled per-check via config.yaml policy.checks:
//
//	continue_on_fail: false (default) — stop immediately, skip remaining checks
//	continue_on_fail: true            — record failure, continue to next check
//
// All failed check names are returned; empty slice means token can be issued.
func RunGate(in Input) []string {
	checks := []check{
		{"repo_role", "Repo role", func() (string, bool) { return checkRepoRole(in) }},
		{"ownership", "Ownership", func() (string, bool) { return checkCodeowners(in, in.Branch) }},
		{"security_scan", "Security scan", func() (string, bool) { return checkScan(in) }},
		{"servicenow_cr", "ServiceNow CR", func() (string, bool) { return checkServiceNow(in) }},
		{"env_promotion", "Env promotion", func() (string, bool) { return checkEnvOrder(in) }},
	}

	total := len(checks)
	var failed []string
	for i, c := range checks {
		detail, ok := c.run()
		if ok {
			fmt.Printf("  [%d/%d] %-13s  ✅ %s\n", i+1, total, c.name, detail)
		} else {
			continueOnFail := config.Global.Policy.ContinueOnFail(c.key)
			tag := ""
			if continueOnFail {
				tag = " (continuing)"
			}
			fmt.Printf("  [%d/%d] %-13s  ❌ %s%s\n", i+1, total, c.name, detail, tag)
			failed = append(failed, strings.TrimSpace(c.name))
			if !continueOnFail {
				// Record skipped checks so the developer knows they were not evaluated.
				for j := i + 1; j < total; j++ {
					fmt.Printf("  [%d/%d] %-13s  ⏭  skipped\n", j+1, total, checks[j].name)
				}
				break
			}
		}
	}
	return failed
}

// ── Check 1: Repo role ────────────────────────────────────────────────────────

func checkRepoRole(in Input) (string, bool) {
	url := fmt.Sprintf(
		"%s/repos/%s/collaborators/%s/permission",
		config.Global.GitHub.APIURL(), in.Repo, in.Identity.Login)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "request build error", false
	}
	req.SetBasicAuth(in.Identity.Login, in.Identity.Token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "GitHub API unreachable", false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("HTTP %d from GitHub", resp.StatusCode), false
	}

	var result struct {
		Permission string `json:"permission"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "decode error", false
	}

	switch result.Permission {
	case "admin", "write":
		return result.Permission, true
	default:
		return fmt.Sprintf("%s (need write or admin)", result.Permission), false
	}
}

// ── Check 2: CODEOWNERS ───────────────────────────────────────────────────────

func checkCodeowners(in Input, branch string) (string, bool) {
	// GitHub supports CODEOWNERS in three locations; try each in order.
	paths := []string{"CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"}

	for _, path := range paths {
		content, found := fetchRawFile(in.Repo, path, branch, in.Identity.Login, in.Identity.Token)
		if !found {
			continue
		}
		// CODEOWNERS entries use @login or @org/team notation.
		if strings.Contains(content, "@"+in.Identity.Login) {
			return "CODEOWNERS", true
		}
		return fmt.Sprintf("@%s not listed in CODEOWNERS", in.Identity.Login), false
	}

	// No CODEOWNERS file at all — skip gracefully.
	return "no CODEOWNERS (skipped)", true
}

func fetchRawFile(repo, path, branch, username, password string) (content string, found bool) {
	rawURL := fmt.Sprintf("%s/repos/%s/contents/%s", config.Global.GitHub.APIURL(), repo, path)
	if branch != "" {
		rawURL += "?ref=" + branch
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", false
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		return "", false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", false
	}
	return string(body), true
}

// ── Check 3: Security scan ────────────────────────────────────────────────────
// STUB — always passes. For trial and prod this should call the governance API.
// TODO: replace with real governance API call using config.Global.Governance credentials.

func checkScan(in Input) (string, bool) {
	switch in.Env {
	case "prod", "trial":
		return "passed [stub — replace with governance API for " + in.Env + "]", true
	default:
		return "skipped (dev)", true
	}
}

// ── Check 4: CR approval ──────────────────────────────────────────────────────
// STUB — always passes. For prod a valid approved CR number is required.
// TODO: replace with real governance API call to verify CR is approved.

func checkServiceNow(in Input) (string, bool) {
	if in.Env == "prod" {
		if in.CR == "" {
			return "CRQ number required for prod — pass --crq CHG0012345", false
		}
		return in.CR + " [stub — replace with governance API]", true
	}
	return "skipped (" + in.Env + ")", true
}

// ── Check 5: Environment promotion order ──────────────────────────────────────

func checkEnvOrder(in Input) (string, bool) {
	order := config.Global.Environments.Order
	if len(order) == 0 {
		return "no order configured (skipped)", true
	}

	for i, e := range order {
		if e == in.Env {
			if i == 0 {
				return fmt.Sprintf("%s (first env)", in.Env), true
			}
			return fmt.Sprintf("%s → %s", order[i-1], in.Env), true
		}
	}
	return fmt.Sprintf("'%s' not in environments.order", in.Env), false
}
