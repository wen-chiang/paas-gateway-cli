//go:build ignore

package main

import (
	"fmt"
	"os"

	"github.com/paas/pgate/internal/argocd"
	"github.com/paas/pgate/internal/config"
)

func main() {
	config.Load("")

	toolCfg, err := config.Global.ToolFor("dev", "argocd")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("Server  : %s\n", toolCfg.ServerURL)
	fmt.Printf("User    : %s\n", toolCfg.Username)
	fmt.Printf("TLS skip: %v\n", toolCfg.TLSSkipVerify)

	token, err := argocd.IssueToken("test-app", toolCfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "IssueToken:", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Token received (first 40 chars): %.40s...\n", token)
}
