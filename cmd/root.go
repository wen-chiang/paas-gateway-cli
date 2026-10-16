package cmd

import (
	"fmt"
	"os"

	"github.com/paas/pgate/internal/config"
	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "pgate",
	Short: "PaaS Gate — policy-enforced token issuer for platform tools",
	Long: `pgate authenticates you via GitHub, runs policy checks,
and issues a short-lived scoped token for platform tools (ArgoCD, Vault, etc.).`,
}

// Execute is the entry point called by main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default: ~/.config/pgate/config.yaml or ./config.yaml)")
}

func initConfig() {
	config.Load(cfgFile)
}
