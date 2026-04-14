package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/logger"
)

func newDeployCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy [component]",
		Short: "Deploy ACS components",
		Long: `Deploy ACS components (central, secured-cluster, operator).

Examples:
  roxie deploy central
  roxie deploy secured-cluster
  roxie deploy both
  roxie deploy operator`,
		ValidArgs: []string{"central", "secured-cluster", "both", "all", "operator"},
		Args:      cobra.MaximumNArgs(1),
		RunE:      runDeploy,
	}

	cmd.Flags().BoolVar(&helm, "helm", false, "Deploy using Helm charts instead of operator")
	_ = cmd.Flags().MarkHidden("helm")
	cmd.Flags().BoolVar(&olm, "olm", false, "Deploy operator via OLM (requires OLM installed)")
	cmd.Flags().BoolVar(&konflux, "konflux", false, "Use Konflux images")
	cmd.Flags().BoolVar(&deployOperator, "deploy-operator", true, "Deploy and check operator (set to false to skip operator deployment/checks)")
	cmd.Flags().BoolVar(&portForwarding, "port-forwarding", false, "Enable localhost port-forward for Central")
	cmd.Flags().BoolVar(&pauseReconciliation, "pause-reconciliation", false, "Pause reconciliation after deployment")
	cmd.Flags().StringVar(&overrideFile, "override", "", "Path to YAML file with overrides")
	cmd.Flags().StringArrayVar(&overrideSetExpressions, "set", []string{}, "Set override values (can specify multiple times, e.g., --set foo.bar=val)")
	cmd.Flags().StringVar(&exposure, "exposure", "loadbalancer", "Central exposure backend (loadbalancer, none)")
	cmd.Flags().StringVar(&resources, "resources", "acs-defaults", "Resource sizing preset (acs-defaults, auto, medium, small, ci)")
	cmd.Flags().StringVar(&shell, "shell", "", "Shell to spawn after Central deployment")
	cmd.Flags().StringVar(&envrc, "envrc", "", "Write environment to file instead of spawning sub-shell")
	cmd.Flags().BoolVar(&singleNamespace, "single-namespace", false, "Deploy all components in a single namespace ('stackrox' by default)")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Main image tag to use for deployment (takes precedence over MAIN_IMAGE_TAG environment variable)")
	cmd.Flags().StringSliceVar(&featureFlags, "features", []string{}, "Feature flag settings (e.g., +ROX_FOO,-ROX_BAR,ROX_BAZ=true)")

	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	// Validate flag combinations early, before env initialization
	if helm && olm {
		return errors.New("cannot use both --helm and --olm flags together")
	}

	if helm && len(featureFlags) > 0 {
		return errors.New("--features flag is not supported with --helm (feature flags only work with operator-based deployments)")
	}

	log := logger.New()
	if err := env.Initialize(log); err != nil {
		return err
	}

	if env.RunningInteractively {
		log.Dim("Running with a controlling terminal.")
	} else {
		log.Dim("Running without a controlling terminal.")
	}

	components, err := component.FromArgs(args)
	if err != nil {
		return err
	}

	if components.IncludesOperatorExplicitly() && helm {
		return errors.New("cannot use --helm flag with 'operator' component")
	}

	if components.IncludesCentral() && os.Getenv("ROXIE_SHELL") != "" {
		return errors.New("already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again")
	}

	if components.IncludesCentral() && !env.RunningInteractively && envrc == "" {
		return errors.New("running without a controlling terminal requires --envrc to be set")
	}

	if envrc != "" && portForwarding {
		return errors.New("cannot use --envrc with --port-forwarding. The --envrc flag is for non-interactive mode with remote cluster access")
	}

	if envrc != "" && exposure == "none" {
		return errors.New("cannot use --envrc with --exposure=none. The --envrc flag requires a remotely accessible endpoint (e.g., --exposure=loadbalancer)")
	}

	portForwardEnabledFinal := portForwarding || exposure == "none"

	if env.RunningInRoxieContainer {
		// For running containerized we have specific requirements.
		if portForwardEnabledFinal {
			return errors.New("containerized mode does not support port-forwarding")
		}
		if exposure == "none" {
			return errors.New("containerized mode requires Central exposure")
		}

		// On infra OpenShift we already get image pull secrets for Quay automatically.
		if clusterType := env.GetCurrentClusterType(); clusterType != env.InfraOpenShift4 {
			if os.Getenv("REGISTRY_USERNAME") == "" || os.Getenv("REGISTRY_PASSWORD") == "" {
				return fmt.Errorf("containerized mode requires REGISTRY_USERNAME and REGISTRY_PASSWORD environment variables for clusters of type %s", clusterType)
			}
			if _, err := os.Stat("/kubeconfig"); err != nil {
				return fmt.Errorf("containerized mode requires /kubeconfig file: %w", err)
			}
		}
	}

	if konflux {
		if helm {
			return errors.New("cannot use both --helm and --konflux flags together (Konflux requires operator-based deployment)")
		}
		if olm {
			return errors.New("cannot use both --olm and --konflux flags together (not currently implemented)")
		}
		clusterType := env.GetCurrentClusterType()
		if clusterType != env.InfraOpenShift4 {
			return fmt.Errorf("--konflux flag is only supported on OpenShift 4 clusters (current cluster type: %s)", clusterType.String())
		}
	}

	if !deployOperator && olm {
		return errors.New("cannot use --deploy-operator=false with --olm (OLM requires operator deployment)")
	}

	d, err := deployer.New(log)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}

	if overrideFile != "" {
		var err error
		if components.IncludesBothCentralAndSensor() {
			err = d.SetCombinedOverrideFile(overrideFile)
		} else if components.IncludesCentral() {
			err = d.SetCentralOverrideFile(overrideFile)
		} else if components.IncludesSensor() {
			err = d.SetSecuredClusterOverrideFile(overrideFile)
		}
		if err != nil {
			return fmt.Errorf("failed to set override file: %w", err)
		}
	}

	if len(overrideSetExpressions) > 0 {
		var err error
		if components.IncludesBothCentralAndSensor() {
			err = d.SetCombinedOverrideSetExpressions(overrideSetExpressions)
		} else if components.IncludesCentral() {
			err = d.SetCentralOverrideSetExpressions(overrideSetExpressions)
		} else if components.IncludesSensor() {
			err = d.SetSecuredClusterOverrideSetExpressions(overrideSetExpressions)
		}
		if err != nil {
			return fmt.Errorf("failed to set override set expressions: %w", err)
		}
	}

	if components.IncludesCentral() {
		d.PrintCentralDeploymentSummary()
	}
	if components.IncludesSensor() {
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

	if olm {
		if err := d.SetUseOLM(true); err != nil {
			return err
		}
	}

	if konflux {
		if err := d.SetUseKonflux(true); err != nil {
			return err
		}

	}

	d.SetDeployOperator(deployOperator)

	d.SetVerbose(verbose)
	d.SetEarlyReadiness(earlyReadiness)
	d.SetPortForwardingEnabled(portForwardEnabledFinal)
	d.SetPauseReconciliation(pauseReconciliation)
	d.SetSingleNamespace(singleNamespace)

	var mainImageTag string
	if tag != "" {
		log.Dimf("Using main image tag from --tag flag: %s", tag)
		mainImageTag = tag
	}
	if mainImageTag == "" {
		mainImageTag, err = helpers.LookupMainImageTag(log)
		if err != nil {
			return fmt.Errorf("looking up main image tag: %w", err)
		}
	}
	d.SetMainImageTag(mainImageTag)

	// Parse and set feature flags (these will have highest precedence)
	if err := d.SetFeatureFlags(featureFlags); err != nil {
		return fmt.Errorf("failed to set feature flags: %w", err)
	}

	// Resolve "auto" resources based on cluster type
	resolvedResources := resources
	if resources == "auto" {
		resolvedResources = resolveAutoResources(env.GetCurrentClusterType(), log)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := d.Deploy(ctx, components, resolvedResources, exposure); err != nil {
		return fmt.Errorf("deployment failed: %w", err)
	}

	log.Success("🎉 Deployment complete!")

	// If Central was deployed, wait for it to be ready before entering subshell
	if components.IncludesCentral() {
		d.WaitForCentral(5 * time.Minute)
	}

	if components.IncludesCentral() && envrc == "" {
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
	case env.InfraOpenShift4:
		resolvedResources = "medium"
	case env.InfraGKE:
		resolvedResources = "medium"
	default:
		resolvedResources = "acs-defaults"
	}

	log.Infof("Auto-detected cluster type %s: using resource profile %q", clusterType.String(), resolvedResources)

	return resolvedResources
}
