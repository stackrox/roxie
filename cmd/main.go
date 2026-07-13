package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"

	"dario.cat/mergo"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/paths"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/strvals"
)

var (
	// Global flags
	verbose bool
	shell   string
	envrc   string
	dryRun  bool

	skipUserConfig bool

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
func tryApplyUserDefaults(log *logger.Logger, config *deployer.Config) error {
	path, err := paths.UserConfigPath(true)
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
	rootCmd.PersistentFlags().BoolVar(&skipUserConfig, "skip-user-config", false,
		fmt.Sprintf("Skips reading of user's configuration (%s)", paths.UserConfigPathString()))
	registerFlag(rootCmd, &deploySettingsFromArgs, "config", "Path to YAML config file",
		withPersistent(),
		withShortName("c"),
		withApplyFn("filename", func(config *deployer.Config, filename string) error {
			if filename == "-" {
				filename = "/dev/stdin"
			}
			data, err := os.ReadFile(filename)
			if err != nil {
				return fmt.Errorf("failed to read config file %q: %w", filename, err)
			}
			var configFromFile deployer.Config
			if err := yaml.Unmarshal(data, &configFromFile); err != nil {
				return fmt.Errorf("failed to unmarshal config file %q: %w", filename, err)
			}
			if err := mergo.Merge(config, configFromFile, mergo.WithOverride, mergo.WithoutDereference); err != nil {
				return fmt.Errorf("merging config file %q into deployer Config: %w", filename, err)
			}
			return nil
		}),
	)
	registerFlag(rootCmd, &deploySettingsFromArgs, "set", "Set expressions, e.g. securedCluster.spec.clusterName=sensor",
		withPersistent(),
		withApplyFn("set-expression", func(config *deployer.Config, expr string) error {
			unstructuredPatch, err := strvals.Parse(expr)
			if err != nil {
				return fmt.Errorf("parsing set expression %q: %w", expr, err)
			}
			if _, forbidden := unstructuredPatch["spec"]; forbidden {
				return errors.New("set expression must not set top-level 'spec'; prefix with 'central.' or 'securedCluster.'")
			}
			var patch deployer.Config
			if err := helpers.MapToStruct(unstructuredPatch, &patch); err != nil {
				return err
			}
			if reflect.DeepEqual(patch, deployer.Config{}) {
				return fmt.Errorf("set expression %q had no effect -- typo?", expr)
			}

			if err := mergo.Merge(config, &patch, mergo.WithOverride, mergo.WithoutDereference); err != nil {
				return fmt.Errorf("merging set-expression %q into deployer Config: %w", expr, err)
			}

			return nil
		}),
	)

	rootCmd.AddCommand(newDeployCmd(&deploySettingsFromArgs))
	rootCmd.AddCommand(newTeardownCmd(&deploySettingsFromArgs))
	rootCmd.AddCommand(newShellCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newEnvCmd())
	rootCmd.AddCommand(newLogsCmd())
}
