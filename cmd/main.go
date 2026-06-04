package main

import (
	"fmt"
	"os"

	"dario.cat/mergo"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/paths"
	"gopkg.in/yaml.v3"
)

var (
	// Global flags
	verbose bool
	shell   string
	envrc   string
	dryRun  bool

	globalLogger = logger.New()

	// We need this set up before command line flags are parsed.
	deploySettingsFromArgs = deployer.NewConfig()
)

func main() {
	red := color.New(color.FgRed, color.Bold)
	if err := rootCmd.Execute(); err != nil {
		red.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// If a user config file exists, apply those user defaults on top the
// current config. This essentially means, that the user config can
// override values, which are already initialized in NewConfig().
// Note: the user config should only contain reasonable fields, which
// are not already handled by roxies smart defaulting like cluster-dependent
// resource profiles.
func tryApplyUserDefaults(log *logger.Logger, config *deployer.Config) error {
	path, err := paths.UserConfigPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading user config %q: %w", path, err)
	}
	var userDefaults deployer.Config
	if err := yaml.Unmarshal(data, &userDefaults); err != nil {
		return fmt.Errorf("parsing user config %q: %w", path, err)
	}
	if err := mergo.Merge(config, &userDefaults, mergo.WithOverride, mergo.WithoutDereference); err != nil {
		return fmt.Errorf("merging user config %q: %w", path, err)
	}
	log.Dimf("Applied user config from %s", path)
	return nil
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
	rootCmd.AddCommand(newDeployCmd(&deploySettingsFromArgs))
	rootCmd.AddCommand(newTeardownCmd(&deploySettingsFromArgs))
	rootCmd.AddCommand(newShellCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newEnvCmd())
	rootCmd.AddCommand(newLogsCmd())
}
