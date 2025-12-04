package main

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	verbose                bool
	earlyReadiness         bool
	helm                   bool
	olm                    bool
	portForwarding         bool
	pauseReconciliation    bool
	overrideFile           string
	overrideSetExpressions []string
	exposure               string
	resources              string
	shell                  string
	envrc                  string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		red := color.New(color.FgRed, color.Bold)
		red.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "roxie",
	Short: "roxie - Advanced Cluster Security Deployment Tool",
	Long: `roxie is a fast, developer-friendly CLI to deploy and manage
Red Hat Advanced Cluster Security (ACS) on any Kubernetes/OpenShift cluster.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output (show CRs and Helm values)")
	rootCmd.PersistentFlags().BoolVar(&earlyReadiness, "early-readiness", false, "Only wait for essential workloads (central/sensor), not all workloads")
	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(newTeardownCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newEnvCmd())
	rootCmd.AddCommand(newLogsCmd())
}
