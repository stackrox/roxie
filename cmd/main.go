package main

import (
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/logger"
)

var (
	// Global flags
	verbose bool
	shell   string
	envrc   string
	dryRun  bool

	globalLogger = logger.New()

	// We need this set up before command line flags are parsed.
	deploySettings = deployer.NewConfig()
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
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output (show CRs)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Do not actually modify cluster")
	rootCmd.AddCommand(newDeployCmd(&deploySettings))
	rootCmd.AddCommand(newTeardownCmd(&deploySettings))
	rootCmd.AddCommand(newShellCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newEnvCmd())
	rootCmd.AddCommand(newLogsCmd())
}
