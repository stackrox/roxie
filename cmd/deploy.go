package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/logger"
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
	cmd.Flags().BoolVar(&olm, "olm", false, "Deploy operator via OLM (requires OLM installed)")
	cmd.Flags().BoolVar(&portForwarding, "port-forwarding", false, "Enable localhost port-forward for Central")
	cmd.Flags().StringVar(&overrideFile, "override", "", "Path to YAML file with overrides")
	cmd.Flags().StringArrayVar(&overrideSetExpressions, "set", []string{}, "Set override values (can specify multiple times, e.g., --set foo.bar=val)")
	cmd.Flags().StringVar(&exposure, "exposure", "loadbalancer", "Central exposure backend (loadbalancer, none)")
	cmd.Flags().StringVar(&resources, "resources", "auto", "Resource sizing preset (auto=cluster-based, medium, small)")
	cmd.Flags().StringVar(&shell, "shell", "", "Shell to spawn after Central deployment")
	cmd.Flags().StringVar(&envrc, "envrc", "", "Write environment to file instead of spawning sub-shell")

	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	log := logger.New()

	if env.RunningInContainer {
		log.Dim("Running containerized.")
	}

	component := "both"
	if len(args) > 0 {
		component = args[0]
	}

	if (component == "central" || component == "both") && os.Getenv("ROXIE_SHELL") != "" {
		return errors.New("already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again")
	}

	if envrc != "" && portForwarding {
		return errors.New("cannot use --envrc with --port-forwarding. The --envrc flag is for non-interactive mode with remote cluster access")
	}

	if envrc != "" && exposure == "none" {
		return errors.New("cannot use --envrc with --exposure=none. The --envrc flag requires a remotely accessible endpoint (e.g., --exposure=loadbalancer)")
	}

	portForwardEnabledFinal := portForwarding || exposure == "none"

	if env.RunningInContainer {
		// For running containerized we have specific requirements.
		if portForwardEnabledFinal {
			return errors.New("containerized mode does not support port-forwarding")
		}
		if exposure == "none" {
			return errors.New("containerized mode requires Central exposure")
		}

		// On infra OpenShift we already get image pull secrets for Quay automatically.
		if env.GetCurrentClusterType() != env.InfraOpenShift4 {
			if os.Getenv("REGISTRY_USERNAME") == "" || os.Getenv("REGISTRY_PASSWORD") == "" {
				return errors.New("containerized mode requires REGISTRY_USERNAME and REGISTRY_PASSWORD environment variables")
			}
			if _, err := os.Stat("/kubeconfig"); err != nil {
				return fmt.Errorf("containerized mode requires /kubeconfig file: %w", err)
			}
		}
		log.Dim("Using KUBECONFIG=/kubeconfig.")
		os.Setenv("KUBECONFIG", "/kubeconfig")
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

	if helm && olm {
		return errors.New("cannot use both --helm and --olm flags together")
	}

	if helm {
		if err := d.SetUseHelm(true); err != nil {
			return err
		}
	}

	if olm {
		if err := d.SetUseOLM(true); err != nil {
			return err
		}
	}

	d.SetVerbose(verbose)
	d.SetEarlyReadiness(earlyReadiness)
	d.SetPortForwardingEnabled(portForwardEnabledFinal)

	// Resolve "auto" resources based on cluster type
	resolvedResources := resources
	if resources == "auto" {
		resolvedResources = resolveAutoResources(env.GetCurrentClusterType(), log)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := d.Deploy(ctx, component, resolvedResources, exposure); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	log.Success("🎉 Deployment complete!")

	// If Central was deployed, wait for it to be ready before entering subshell
	if component == "central" || component == "both" || component == "all" {
		d.WaitForCentral(5 * time.Minute)
	}

	if (component == "central" || component == "both") && envrc == "" {
		if err := spawnSubshell(d, log); err != nil {
			return fmt.Errorf("failed to spawn subshell: %w", err)
		}
	}

	return nil
}

// resolveAutoResources determines the appropriate resource tier based on cluster type
func resolveAutoResources(clusterType env.ClusterType, log *logger.Logger) string {
	var resolvedResources string

	switch clusterType {
	case env.LocalKind:
		resolvedResources = "small"
		log.Info("Auto-detected cluster type Kind: using small resources")
	case env.InfraOpenShift4:
		resolvedResources = "medium"
		log.Info("Auto-detected cluster type OpenShift 4: using medium resources")
	default:
		resolvedResources = "default"
		log.Info("Auto-detected cluster type " + clusterType.String() + ": using default resources")
	}

	return resolvedResources
}
