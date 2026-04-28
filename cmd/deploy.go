package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stackrox/roxie/internal/clusterdefaults"
	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/types"

	"github.com/stackrox/roxie/internal/stackroxversions"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"
)

var (
	sharedNamespace = "stackrox"
)

// For extended short-cut parameters.
type configShortCut struct {
	settings *deployer.Config
	flagType string
	applyFn  func(val string, settings *deployer.Config) error
}

func newConfigShortCut(
	settings *deployer.Config,
	flagType string,
	applyFn func(val string, settings *deployer.Config) error,
) *configShortCut {
	return &configShortCut{
		flagType: flagType,
		settings: settings,
		applyFn:  applyFn,
	}
}

func (y *configShortCut) Set(val string) error {
	return y.applyFn(val, y.settings)
}

func (y *configShortCut) String() string {
	return "" // Not sure what to return here.
}

func (y *configShortCut) Type() string {
	return y.flagType
}

func newConfigShortCutBool(settings *deployer.Config, path string) *configShortCut {
	pathElements := strings.Split(path, ".")
	applyFn := func(val string, settings *deployer.Config) error {
		var valParsed bool
		if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
			return err
		}
		u, err := helpers.StructToMap(settings)
		if err != nil {
			return err
		}
		if err := unstructured.SetNestedField(u, valParsed, pathElements...); err != nil {
			return err
		}
		return helpers.MapToStruct(u, settings)
	}
	return newConfigShortCut(settings, "bool", applyFn)
}

func newDeployCmd(settings *deployer.Config) *cobra.Command {
	var flag *pflag.Flag

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

	// --shell <shell name>.
	cmd.Flags().StringVar(&shell, "shell", "", "Shell to spawn after Central deployment")

	// --envrc <filename>.
	cmd.Flags().StringVar(&envrc, "envrc", "", "Write environment to file instead of spawning sub-shell")

	// --olm[=true/false].
	flag = cmd.Flags().VarPF(newConfigShortCutBool(settings, "operator.deployViaOlm"), "olm", "", "Deploy operator via OLM (requires OLM installed)")
	flag.NoOptDefVal = "true"

	// --konflux[=true/false]
	flag = cmd.Flags().VarPF(newConfigShortCutBool(settings, "roxie.konfluxImages"), "konflux", "", "Use Konflux images")
	flag.NoOptDefVal = "true"

	// --deploy-operator[=true/false].
	flag = cmd.Flags().VarPF(
		newConfigShortCut(
			settings,
			"bool",
			func(val string, settings *deployer.Config) error {
				var valParsed bool
				if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
					return err
				}
				settings.Operator.SkipDeployment = !valParsed
				return nil
			},
		), "deploy-operator", "", "Whether to deploy and manage the operator")
	flag.NoOptDefVal = "true"

	// --port-forward[=true/false].
	flag = cmd.Flags().VarPF(
		newConfigShortCut(
			settings, "bool",
			func(val string, settings *deployer.Config) error {
				var valParsed bool
				if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
					return err
				}
				settings.Central.PortForwarding = ptr.To(valParsed)
				return nil
			},
		), "port-forwarding", "", "Enable localhost port-forward for Central")
	flag.NoOptDefVal = "true"

	// --pause-reconciliation[=true/false].
	flag = cmd.Flags().VarPF(
		newConfigShortCut(
			settings, "bool",
			func(val string, settings *deployer.Config) error {
				var valParsed bool
				if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
					return err
				}
				settings.Central.PauseReconciliation = valParsed
				settings.SecuredCluster.PauseReconciliation = valParsed
				return nil
			},
		), "pause-reconciliation", "", "Pause reconciliation after deployment")
	flag.NoOptDefVal = "true"

	// --config/-c <filename>.
	cmd.Flags().VarP(
		newConfigShortCut(
			settings, "file",
			func(filename string, settings *deployer.Config) error {
				if filename == "-" {
					filename = "/dev/stdin"
				}
				data, err := os.ReadFile(filename)
				if err != nil {
					return fmt.Errorf("failed to read config file %q: %w", filename, err)
				}
				var obj map[string]interface{}
				if err := yaml.Unmarshal(data, &obj); err != nil {
					return fmt.Errorf("failed to decode config file %q: %w", filename, err)
				}
				return settings.MergeInUnstructured(obj)
			},
		), "config", "c", "Path to YAML config file")

	// --exposure loadbalancer/none.
	cmd.Flags().Var(
		newConfigShortCut(
			settings, "exposure",
			func(val string, settings *deployer.Config) error {
				var exposure types.Exposure
				if err := yaml.Unmarshal([]byte(val), &exposure); err != nil {
					return err
				}
				settings.Central.Exposure = exposure
				return nil
			},
		), "exposure", "Central exposure backend (loadbalancer, none)")

	// --resources <resource profile name>.
	cmd.Flags().Var(
		newConfigShortCut(
			settings, "resource-profile",
			func(val string, settings *deployer.Config) error {
				var valParsed types.ResourceProfile
				if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
					return err
				}
				settings.Central.ResourceProfile = valParsed
				settings.SecuredCluster.ResourceProfile = valParsed
				return nil
			},
		), "resources", fmt.Sprintf("Resource sizing preset (%s)", types.ResourceProfilesJoined()))

	// --set <set expressions>.
	cmd.Flags().Var(newConfigShortCut(settings, "set-expression",
		func(expr string, settings *deployer.Config) error {
			key, yamlValue, found := strings.Cut(expr, "=")
			if !found {
				return fmt.Errorf("invalid set expression '%s': expected format 'key.path=value'", expr)
			}
			var val interface{}
			if err := yaml.Unmarshal([]byte(yamlValue), &val); err != nil {
				return fmt.Errorf("failed to unmarshal value '%s' for key '%s': %w", yamlValue, key, err)
			}
			// SetNestedField requires JSON-compatible types: float64 for numbers, not int.
			switch v := val.(type) {
			case int:
				val = float64(v)
			case int64:
				val = float64(v)
			}
			pathElements := strings.Split(key, ".")
			if len(pathElements) > 0 && pathElements[0] == "spec" {
				// Special error reporting for this case, because it was supported previously.
				return errors.New("set expression begin with 'spec.' -- it must be prefixed with 'central.' or 'securedCluster.'")
			}
			u, err := helpers.StructToMap(settings)
			if err != nil {
				return err
			}
			if err := unstructured.SetNestedField(u, val, pathElements...); err != nil {
				return err
			}
			var updatedSettings deployer.Config
			if err := helpers.MapToStruct(u, &updatedSettings); err != nil {
				return err
			}
			if reflect.DeepEqual(settings, &updatedSettings) {
				return fmt.Errorf("Set expression %q had no effect -- typo?", expr)
			}
			*settings = updatedSettings

			return nil

		},
	), "set", "Set expressions, e.g. securedCluster.spec.clusterName=sensor")

	// --single-namespace[=true/false].
	flag = cmd.Flags().VarPF(
		newConfigShortCut(
			settings, "bool",
			func(val string, settings *deployer.Config) error {
				var valParsed bool
				if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
					return err
				}
				if valParsed {
					settings.Central.Namespace = sharedNamespace
					settings.SecuredCluster.Namespace = sharedNamespace
				}
				return nil
			},
		), "single-namespace", "", "Deploy all components in a single namespace ('stackrox')")
	flag.NoOptDefVal = "true"

	// --tag/-t <main image tag>.
	cmd.Flags().VarP(
		newConfigShortCut(
			settings, "version",
			func(mainImageTag string, settings *deployer.Config) error {
				settings.Roxie.Version = mainImageTag
				return nil
			},
		), "tag", "t", "Main image tag to use for deployment (takes precedence over MAIN_IMAGE_TAG environment variable)")

	// --features <feature flags...>
	cmd.Flags().Var(
		newConfigShortCut(
			settings, "feature-flags",
			func(featureFlagExpr string, settings *deployer.Config) error {
				featureFlags, err := deployer.ParseFeatureFlags([]string{featureFlagExpr})
				if err != nil {
					return fmt.Errorf("parsing feature flags: %w", err)
				}
				for k, v := range featureFlags {
					settings.Roxie.FeatureFlags[k] = v
				}
				return nil
			},
		), "features", "Feature flag settings (e.g., +ROX_FOO,-ROX_BAR,ROX_BAZ=true)")

	// --central-wait <duration>.
	cmd.Flags().Var(
		newConfigShortCut(
			settings, "duration",
			func(val string, settings *deployer.Config) error {
				duration, err := time.ParseDuration(val)
				if err != nil {
					return err
				}
				settings.Central.DeployTimeout = duration
				return nil
			},
		), "central-wait", "maximum wait time for central to become ready (e.g., 5m, 10m)")

	// --secured-cluster-wait <duration>.
	cmd.Flags().Var(
		newConfigShortCut(
			settings, "duration",
			func(val string, settings *deployer.Config) error {
				duration, err := time.ParseDuration(val)
				if err != nil {
					return err
				}
				settings.SecuredCluster.DeployTimeout = duration
				return nil
			},
		), "secured-cluster-wait", "maximum wait time for secured cluster to become ready (e.g., 5m, 10m)")

	// --early-readiness[=true/false].
	flag = cmd.Flags().VarPF(
		newConfigShortCut(
			settings, "bool",
			func(val string, settings *deployer.Config) error {
				var valParsed bool
				if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
					return err
				}
				if valParsed {
					settings.Central.EarlyReadiness = true
					settings.SecuredCluster.EarlyReadiness = true
				}
				return nil
			},
		), "early-readiness", "", "Only wait for essential workloads (central/sensor) to be ready")
	flag.NoOptDefVal = "true"

	// Make --override an alias for --config, for backwards compatibility.
	cmd.Flags().SetNormalizeFunc(func(f *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "override" {
			name = "config"
		}
		return pflag.NormalizedName(name)
	})

	return cmd
}

func runDeploy(cmd *cobra.Command, args []string) error {
	log := logger.New()
	if !dryRun {
		if err := env.Initialize(log); err != nil {
			return err
		}
	}

	if env.RunningInteractively {
		log.Dim("Running with a controlling terminal.")
	} else {
		log.Dim("Running without a controlling terminal.")
	}

	clusterType := env.GetCurrentClusterType()
	log.Dimf("Detected cluster type: %v", clusterType)
	err := clusterdefaults.ApplyClusterDefaults(log, clusterType, &deploySettings)
	if err != nil {
		return fmt.Errorf("applying defaults for cluster type %v: %w", clusterType, err)
	}

	// Deal with the "auto" resourceProfile.
	if deploySettings.Central.ResourceProfile == types.ResourceProfileAuto {
		profile := clusterdefaults.ResolveAutoResourceProfile(clusterType)
		log.Dimf("Selecting resource profile %v for Central", profile)
		deploySettings.Central.ResourceProfile = profile
	}
	if deploySettings.SecuredCluster.ResourceProfile == types.ResourceProfileAuto {
		profile := clusterdefaults.ResolveAutoResourceProfile(clusterType)
		deploySettings.SecuredCluster.ResourceProfile = profile
	}

	components, err := component.FromArgs(args)
	if err != nil {
		return err
	}

	if deploySettings.Roxie.Version != "" {
		log.Dimf("Using main image tag %s", deploySettings.Roxie.Version)
	} else {
		mainImageTag, err := helpers.LookupMainImageTag(log)
		if err != nil {
			return fmt.Errorf("looking up main image tag: %w", err)
		}
		deploySettings.Roxie.Version = mainImageTag
	}

	if !deploySettings.Operator.SkipDeployment {
		if err := deploySettings.Operator.Configure(&deploySettings.Roxie); err != nil {
			return fmt.Errorf("configuring operator configuration: %w", err)
		}
	}

	if verbose {
		log.Dim("Deployment configuration:")
		helpers.LogMultilineYaml(log, deploySettings)
	}

	if components.IncludesCentral() {
		if err := deploySettings.Central.ConfigureSpec(&deploySettings.Roxie); err != nil {
			return fmt.Errorf("configuring Central spec: %w", err)
		}
	}
	if components.IncludesSensor() {
		if err := deploySettings.SecuredCluster.ConfigureSpec(&deploySettings.Roxie, &deploySettings.Central); err != nil {
			return fmt.Errorf("configuring SecuredCluster spec: %w", err)
		}
	}

	if components.IncludesCentral() && os.Getenv("ROXIE_SHELL") != "" {
		return errors.New("already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again")
	}

	if components.IncludesCentral() && !env.RunningInteractively && envrc == "" {
		return errors.New("running without a controlling terminal requires --envrc to be set")
	}

	if envrc != "" && deploySettings.Central.PortForwardingEnabled() {
		return errors.New("cannot use --envrc with central port-forwarding enabled. The --envrc flag is for non-interactive mode with remote cluster access")
	}

	if envrc != "" && deploySettings.Central.Exposure == types.ExposureNone {
		return errors.New("cannot use --envrc with --exposure=none. The --envrc flag requires a remotely accessible endpoint (e.g., --exposure=loadbalancer)")
	}

	if !deploySettings.Central.PortForwardingSet() && deploySettings.Central.Exposure == types.ExposureNone {
		log.Info("Enabling port-forwarding due to no exposure")
		deploySettings.Central.PortForwarding = ptr.To(true)
	}

	if env.RunningInRoxieContainer {
		// For running containerized we have specific requirements.
		if deploySettings.Central.PortForwardingEnabled() {
			return errors.New("containerized mode does not support port-forwarding")
		}
		if deploySettings.Central.Exposure == types.ExposureNone {
			return errors.New("containerized mode requires Central exposure")
		}

		// On infra OpenShift we already get image pull secrets for Quay automatically.
		if clusterType := env.GetCurrentClusterType(); clusterType != types.ClusterTypeInfraOpenShift4 {
			if os.Getenv("REGISTRY_USERNAME") == "" || os.Getenv("REGISTRY_PASSWORD") == "" {
				return fmt.Errorf("containerized mode requires REGISTRY_USERNAME and REGISTRY_PASSWORD environment variables for clusters of type %s", clusterType)
			}
			if _, err := os.Stat("/kubeconfig"); err != nil {
				return fmt.Errorf("containerized mode requires /kubeconfig file: %w", err)
			}
		}
	}

	if deploySettings.Roxie.KonfluxImages {
		if deploySettings.Operator.DeployViaOlm {
			return errors.New("using Konflux images while deploying operator via OLM is not supported")
		}
		clusterType := env.GetCurrentClusterType()
		if clusterType != types.ClusterTypeInfraOpenShift4 {
			return fmt.Errorf("--konflux flag is only supported on OpenShift 4 clusters (current cluster type: %s)", clusterType.String())
		}
	}

	if deploySettings.Operator.SkipDeployment && deploySettings.Operator.DeployViaOlm {
		return errors.New("skipping operator deployment while also requesting deploying via OLM at the same time does not make sense")
	}

	if !deploySettings.Central.EarlyReadiness || !deploySettings.SecuredCluster.EarlyReadiness {
		hasSupport, err := stackroxversions.SupportsAdditionalPrinterColumns(deploySettings.Operator.Version)
		if err != nil {
			return fmt.Errorf("checking version constraint on main image tag %s", deploySettings.Roxie.Version)
		}
		if !hasSupport {
			return fmt.Errorf("--early-readiness=false can only by used for StackRox versions satisfying %s", stackroxversions.SupportsAdditionalPrinterColumnsConstraint.String())
		}
	}

	d, err := deployer.New(log)
	if err != nil {
		return fmt.Errorf("failed to create deployer: %w", err)
	}
	defer d.Cleanup()

	if envrc != "" {
		d.SetEnvrcFile(envrc)
	}

	d.SetVerbose(verbose)

	if dryRun {
		log.Info("Exiting because of enabled dry run mode.")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	d.SetConfig(deploySettings)
	if components.IncludesCentral() {
		d.PrintCentralDeploymentSummary()
	}
	if components.IncludesSensor() {
		d.PrintSecuredClusterDeploymentSummary()
	}

	if err := d.Deploy(ctx, components); err != nil {
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
