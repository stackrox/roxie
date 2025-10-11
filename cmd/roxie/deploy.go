package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie-golang/pkg/deployer"
	"github.com/stackrox/roxie-golang/pkg/logger"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [component]",
		Short: "Deploy ACS components",
		Long: `Deploy ACS components (central, secured-cluster).

Examples:
  roxie deploy central
  roxie deploy secured-cluster
  roxie deploy both
  roxie deploy central --helm`,
		ValidArgs: []string{"central", "secured-cluster", "both", "all"},
		Args:      cobra.MaximumNArgs(1),
		RunE:      runDeploy,
	}

	cmd.Flags().BoolVar(&helm, "helm", false, "Deploy using Helm charts instead of operator")
	cmd.Flags().BoolVar(&portForwarding, "port-forwarding", false, "Enable localhost port-forward for Central")
	cmd.Flags().StringVar(&overrideFile, "override", "", "Path to YAML file with overrides")
	cmd.Flags().StringArrayVar(&overrideSetExpressions, "set", []string{}, "Set override values (can specify multiple times, e.g., --set foo.bar=val)")
	cmd.Flags().StringVar(&exposure, "exposure", "loadbalancer", "Central exposure backend (loadbalancer, none)")
	cmd.Flags().StringVar(&resources, "resources", "default", "Resource sizing preset (default, small)")
	cmd.Flags().StringVar(&shell, "shell", "", "Shell to spawn after Central deployment")
	cmd.Flags().StringVar(&envrc, "envrc", "", "Write environment to file instead of spawning sub-shell")

	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	component := "both"
	if len(args) > 0 {
		component = args[0]
	}

	log := logger.New()

	if (component == "central" || component == "both") && os.Getenv("ROXIE_SHELL") != "" {
		return errors.New("already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again")
	}

	if envrc != "" && portForwarding {
		return errors.New("cannot use --envrc with --port-forwarding. The --envrc flag is for non-interactive mode with remote cluster access")
	}

	if envrc != "" && exposure == "none" {
		return errors.New("cannot use --envrc with --exposure=none. The --envrc flag requires a remotely accessible endpoint (e.g., --exposure=loadbalancer)")
	}

	d, err := deployer.New(log, overrideFile, overrideSetExpressions)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	switch component {
	case "central", "both", "all":
		d.PrintCentralDeploymentSummary()
	case "secured-cluster", "sensor":
		d.PrintSecuredClusterDeploymentSummary()
	}

	if envrc != "" {
		d.SetEnvrcFile(envrc)
	}

	if helm {
		if err := d.SetUseHelm(true); err != nil {
			return err
		}
	}

	d.SetVerbose(verbose)
	d.SetEarlyReadiness(earlyReadiness)

	portForwardEnabledFinal := portForwarding || exposure == "none"
	d.SetPortForwardingEnabled(portForwardEnabledFinal)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := d.Deploy(ctx, component, resources, exposure); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	log.Success("🎉 Deployment complete!")

	if (component == "central" || component == "both") && envrc == "" {
		if err := spawnSubshell(d, log); err != nil {
			return fmt.Errorf("failed to spawn subshell: %w", err)
		}
	}

	return nil
}
