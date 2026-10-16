package config

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// CheckPolicy controls how a single policy check behaves on failure.
type CheckPolicy struct {
	// ContinueOnFail: if true, a failure records the check as failed but
	// execution continues to the next check.  If false (default), the gate
	// stops immediately and skips all remaining checks.
	ContinueOnFail bool `mapstructure:"continue_on_fail"`
}

// PolicyConfig holds per-check behaviour, loaded from config.yaml under "policy.checks".
type PolicyConfig struct {
	Checks map[string]CheckPolicy `mapstructure:"checks"`
}

// ContinueOnFail returns the continue_on_fail value for a named check.
// Defaults to false (stop on first failure) when the check key is absent.
func (p PolicyConfig) ContinueOnFail(checkKey string) bool {
	if cp, ok := p.Checks[checkKey]; ok {
		return cp.ContinueOnFail
	}
	return false
}

// GitHubConfig holds GitHub connection settings.
type GitHubConfig struct {
	// BaseURL is the root URL of your GitHub instance.
	// Use "https://github.com" for GitHub.com, or your GHE host e.g. "https://github.sampleOrg.com".
	BaseURL string `mapstructure:"base_url"`
}

// WebURL returns the base web URL, defaulting to https://github.com.
func (g GitHubConfig) WebURL() string {
	if g.BaseURL != "" {
		return strings.TrimRight(g.BaseURL, "/")
	}
	return "https://github.com"
}

// APIURL returns the REST API base URL.
// GitHub Enterprise Server exposes the API at <host>/api/v3.
// GitHub.com uses https://api.github.com.
func (g GitHubConfig) APIURL() string {
	if g.BaseURL != "" && g.BaseURL != "https://github.com" {
		return strings.TrimRight(g.BaseURL, "/") + "/api/v3"
	}
	return "https://api.github.com"
}

// GlobalConfig is loaded from config.yaml (global / platform-wide settings).
type GlobalConfig struct {
	GitHub GitHubConfig `mapstructure:"github"`

	Environments struct {
		// Order defines the allowed promotion chain, e.g. [dev, staging, prod]
		Order []string `mapstructure:"order"`
	} `mapstructure:"environments"`

	Policy PolicyConfig `mapstructure:"policy"`

	// Profiles maps environment name → tool name → tool connection config.
	// Example: Profiles["dev"]["argocd"].ServerURL
	// This is the source of truth for all tool credentials per environment.
	// Future: replace with a fetch from a central config server.
	Profiles map[string]map[string]ToolConfig `mapstructure:"profiles"`

	// Repos is a static fallback mapping when a repo has no .pgate.yaml.
	Repos map[string]RepoMapping `mapstructure:"repos"`
}

// ToolConfig holds the connection details for any platform tool in a profile.
type ToolConfig struct {
	ServerURL string `mapstructure:"server_url"`
	// TLSSkipVerify disables TLS certificate verification.
	// Use true for local/dev environments with self-signed certificates.
	TLSSkipVerify bool `mapstructure:"tls_skip_verify"`
	// GRPCWeb uses gRPC-Web (HTTP/1.1) instead of native gRPC (HTTP/2).
	// Required when ArgoCD is behind an ingress or proxy that does not support HTTP/2.
	GRPCWeb bool `mapstructure:"grpc_web"`
}

// ToolFor returns the ToolConfig for a given profile (env) and tool name.
// Returns a clear error if the profile or tool is not defined in config.yaml.
func (g GlobalConfig) ToolFor(profile, tool string) (ToolConfig, error) {
	tools, ok := g.Profiles[profile]
	if !ok {
		return ToolConfig{}, fmt.Errorf(
			"profile %q not found in config.yaml\n"+
				"  → Add it under profiles: (see config.yaml for examples)",
			profile)
	}
	cfg, ok := tools[tool]
	if !ok {
		return ToolConfig{}, fmt.Errorf(
			"tool %q is not configured for profile %q\n"+
				"  → Add it under profiles.%s.%s in config.yaml",
			tool, profile, profile, tool)
	}
	if cfg.ServerURL == "" {
		return ToolConfig{}, fmt.Errorf(
			"profiles.%s.%s.server_url is not set in config.yaml", profile, tool)
	}
	return cfg, nil
}

// RepoMapping is the static per-repo section inside config.yaml.
type RepoMapping struct {
	EnvApps map[string]string `mapstructure:"env_apps"`
}

// RepoConfig is loaded from .pgate.yaml committed in each application repo.
type RepoConfig struct {
	AppName string            `yaml:"app_name"`
	EnvApps map[string]string `yaml:"env_apps"`
}

// Global holds the loaded platform config, accessible by all packages.
var Global GlobalConfig

// Load reads config.yaml into Global. Called once at startup.
func Load(cfgFile string) {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("$HOME/.config/pgate")
		viper.AddConfigPath(".")
	}
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
	_ = viper.Unmarshal(&Global)
}

// LoadRepoConfig fetches .pgate.yaml from the repo root via GitHub raw API.
// branch may be empty, in which case the repo's default branch is used.
// Returns an empty (non-nil) RepoConfig if the file does not exist or cannot be read.
func LoadRepoConfig(repo, branch, username, password string) *RepoConfig {
	rawURL := fmt.Sprintf("%s/repos/%s/contents/.pgate.yaml", Global.GitHub.APIURL(), repo)
	if branch != "" {
		rawURL += "?ref=" + branch
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return &RepoConfig{}
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Accept", "application/vnd.github.v3.raw")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &RepoConfig{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &RepoConfig{}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &RepoConfig{}
	}

	var rc RepoConfig
	_ = yaml.Unmarshal(body, &rc)
	return &rc
}

// ResolveAppName returns the ArgoCD application name for repo+env.
// Priority order:
//  1. Per-repo .pgate.yaml  env_apps[env]
//  2. Per-repo .pgate.yaml  app_name + "-" + env
//  3. Global config.yaml    repos[repo].env_apps[env]
//  4. Derived:              last segment of repo + "-" + env
func ResolveAppName(repo, env string, rc *RepoConfig) string {
	if rc != nil {
		if name, ok := rc.EnvApps[env]; ok && name != "" {
			return name
		}
		if rc.AppName != "" {
			return rc.AppName + "-" + env
		}
	}
	if m, ok := Global.Repos[repo]; ok {
		if name, ok2 := m.EnvApps[env]; ok2 && name != "" {
			return name
		}
	}
	// Fallback: "myorg/payment-service" → "payment-service-staging"
	parts := strings.SplitN(repo, "/", 2)
	return parts[len(parts)-1] + "-" + env
}
