package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"dario.cat/mergo"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stackrox/roxie/internal/clusterdefaults"
	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/manifest"
	"github.com/stackrox/roxie/internal/roxieenv"
	"github.com/stackrox/roxie/internal/stackroxversions"
	"github.com/stackrox/roxie/internal/types"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/strvals"
)

var (
	sharedNamespace     = "stackrox"
	imagePreLoadCommand string
)

func newDeployCmd(settings *deployer.Config) *cobra.Command {
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

	cmd.Flags().StringVar(&shell, "shell", "", "Shell to spawn after Central deployment")

	cmd.Flags().StringVar(&envrc, "envrc", "", "Write environment to file instead of spawning sub-shell")
	cmd.Flags().StringVar(&imagePreLoadCommand, "image-preload-command", "",
		`Use custom command for pre-loading images to local cluster. Image can be referenced as $IMAGE.
Pre-loading support for Kind and Minikube clusters is built-in. In case this is not sufficient
this flag can be used to tell roxie how to pre-load images for the current cluster.`)

	registerFlag(cmd, settings, "olm", "Deploy operator via OLM (requires OLM installed)",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			config.Operator.DeployViaOlm = new(val)
			return nil
		}),
	)

	registerFlag(cmd, settings, "konflux", "Use Konflux images",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			config.Roxie.KonfluxImages = new(val)
			return nil
		}),
	)

	registerFlag(cmd, settings, "deploy-operator", "Whether to deploy and manage the operator",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			config.Operator.SkipDeployment = new(!val)
			return nil
		}),
	)

	registerFlag(cmd, settings, "port-forwarding", "Enable localhost port-forward for Central",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			config.Central.PortForwarding = new(val)
			return nil
		}),
	)

	registerFlag(cmd, settings, "pause-reconciliation", "Pause reconciliation after deployment",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			config.Central.PauseReconciliation = new(val)
			config.SecuredCluster.PauseReconciliation = new(val)
			return nil
		}),
	)

	registerFlag(cmd, settings, "config", "Path to YAML config file",
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

	registerFlag(cmd, settings, "exposure", "Central exposure backend (loadbalancer, none)",
		withApplyFn("exposure", func(config *deployer.Config, val string) error {
			var exposure types.Exposure
			if err := yaml.Unmarshal([]byte(val), &exposure); err != nil {
				return err
			}
			config.Central.Exposure = new(exposure)
			return nil
		}),
	)

	registerFlag(cmd, settings, "resources", fmt.Sprintf("Resource sizing preset (%s)", types.ResourceProfilesJoined()),
		withApplyFn("resource-profile", func(config *deployer.Config, val string) error {
			var valParsed types.ResourceProfile
			if err := yaml.Unmarshal([]byte(val), &valParsed); err != nil {
				return err
			}
			config.Central.ResourceProfile = valParsed
			config.SecuredCluster.ResourceProfile = valParsed
			return nil
		}),
	)

	registerFlag(cmd, settings, "set", "Set expressions, e.g. securedCluster.spec.clusterName=sensor",
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

	registerFlag(cmd, settings, "single-namespace", "Deploy all components in a single namespace ('stackrox')",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			// We do not support --single-namespace=false as of now.
			if val {
				config.Central.Namespace = sharedNamespace
				config.SecuredCluster.Namespace = sharedNamespace
			}
			return nil
		}),
	)

	registerFlag(cmd, settings, "tag", "Main image tag to use for deployment (takes precedence over MAIN_IMAGE_TAG environment variable)",
		withShortName("t"),
		withApplyFn("version", func(config *deployer.Config, mainImageTag string) error {
			config.Roxie.Version = mainImageTag
			return nil
		}),
	)

	registerFlag(cmd, settings, "operator-env", "Operator environment variables (e.g., RELATED_IMAGE_MAIN=quay.io/...)",
		withApplyFn("env-var", func(config *deployer.Config, envExpr string) error {
			key, value, err := deployer.ParseOperatorEnvVar(envExpr)
			if err != nil {
				return fmt.Errorf("parsing operator env var: %w", err)
			}
			if config.Operator.EnvVars == nil {
				config.Operator.EnvVars = make(map[string]string)
			}
			config.Operator.EnvVars[key] = value
			return nil
		}),
	)

	registerFlag(cmd, settings, "features", "Feature flag settings (e.g., +ROX_FOO,-ROX_BAR,ROX_BAZ=true)",
		withApplyFn("feature-flags", func(config *deployer.Config, featureFlagExpr string) error {
			featureFlags, err := deployer.ParseFeatureFlags([]string{featureFlagExpr})
			if err != nil {
				return fmt.Errorf("parsing feature flags: %w", err)
			}
			for k, v := range featureFlags {
				config.Roxie.FeatureFlags[k] = v
			}
			return nil
		}),
	)

	registerFlag(cmd, settings, "central-wait", "maximum wait time for central to become ready (e.g., 5m, 10m)",
		withApplyFnDuration(func(config *deployer.Config, duration time.Duration) error {
			config.Central.DeployTimeout = duration
			return nil
		}),
	)

	registerFlag(cmd, settings, "secured-cluster-wait", "maximum wait time for secured cluster to become ready (e.g., 5m, 10m)",
		withApplyFnDuration(func(config *deployer.Config, duration time.Duration) error {
			config.SecuredCluster.DeployTimeout = duration
			return nil
		}),
	)

	registerFlag(cmd, settings, "early-readiness", "Only wait for essential workloads (central/sensor) to be ready",
		withNoOptDefVal("true"),
		withApplyFnBool(func(config *deployer.Config, val bool) error {
			config.Central.EarlyReadiness = new(val)
			config.SecuredCluster.EarlyReadiness = new(val)
			return nil
		}),
	)

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
	log := globalLogger
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

	components, err := component.FromArgs(args)
	if err != nil {
		return err
	}

	// Start with default configuration.
	deploySettings := deployer.DefaultConfig()

	// Apply user config on top (overriding defaults).
	if !skipUserConfig {
		if err := tryApplyUserDefaults(globalLogger, &deploySettings); err != nil {
			return fmt.Errorf("applying user config: %w", err)
		}
	}

	// Apply changes from arg parsing.
	if err := mergo.Merge(&deploySettings, &deploySettingsFromArgs, mergo.WithOverride, mergo.WithoutDereference); err != nil {
		return fmt.Errorf("applying config patches from command line argument: %w", err)
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

	if err := configureConfig(log, components, &deploySettings); err != nil {
		return err
	}

	if err := deployValidate(components, &deploySettings); err != nil {
		return err
	}

	if !deploySettings.Central.EarlyReadinessEnabled() || !deploySettings.SecuredCluster.EarlyReadinessEnabled() {
		// Explanation on the versions involved here:
		// Deploying StackRox begins with picking a "main image tag" -- this is a version identifier, which cannot be reliably parsed as a semver.
		// But there is a derived version from that -- the operator version -- which can be parsed as a semver.
		//
		// The invocation of deploySettings.Operator.Configure() above in this function prepares the operator deployment config in the sense
		// that top-level roxie configuration options are propagated to the concrete operator deployment configuration. This includes also
		// storing of the derived operator version within the operator configuration.
		//
		// This is why we use the operator version here when checking version constraints.
		hasSupport, err := stackroxversions.SupportsAdditionalPrinterColumns(deploySettings.Operator.Version)
		if err != nil {
			return fmt.Errorf("checking version constraint on main image tag %s: %w", deploySettings.Roxie.Version, err)
		}
		if !hasSupport {
			return fmt.Errorf("--early-readiness=false can only be used for StackRox versions satisfying %s", stackroxversions.SupportsAdditionalPrinterColumnsConstraint.String())
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
	d.SetConfig(deploySettings)

	if dryRun {
		log.Info("Exiting because of enabled dry run mode.")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// If we are deploying to a local cluster and the images exist locally, then we transfer them
	// to the local cluster.
	if deploySettings.Roxie.ClusterType.IsLocal() && !deploySettings.Roxie.KonfluxImagesEnabled() {
		var preLoader deployer.ImagePreLoader
		if imagePreLoadCommand != "" {
			preLoader = deployer.NewCustomImagePreloader(ctx, log, imagePreLoadCommand)
		} else {
			preLoader, err = d.GetPreLoaderForCluster()
			if err != nil {
				// ErrLocalImagesUnsupported indicates that roxie does not contain preloading
				// support for the respective cluster type. If preloading is required (because
				// the images do not exist on the remote registry), the user needs to take care
				// of the preloading.
				if errors.Is(err, deployer.ErrLocalImagesUnsupported) {
					log.Warningf("Image preloading not supported for cluster %s.", d.GetKubeContext())
					log.Warningf("Use --image-preload-command for specifying custom image preloading mechanism.")
				} else {
					return fmt.Errorf("obtaining image preloader for cluster: %w", err)
				}
			}
		}
		if preLoader != nil {
			log.Dimf("Using image pre-loader %q", preLoader.Name())
			if err := d.TryTransferLocalImages(ctx, preLoader); err != nil {
				// Best effort, keep running.
				log.Warningf("Transferring images to local cluster failed: %v", err)
			}
		}
	}

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

	if components.IncludesCentral() && !dryRun {
		roxieEnv := roxieenv.AssembleRoxieEnvironment(d.GetCentralDeploymentInfo())
		m := manifest.RoxieManifest{
			RoxieEnvironment: roxieEnv,
			Config:           deploySettings,
		}
		if err := manifest.CreateManifestSecretOnCluster(ctx, log, m); err != nil {
			log.Warningf("Failed to save roxie manifest: %v", err)
		}
	}

	if components.IncludesCentral() && envrc == "" {
		if err := spawnSubshellForDeployerEnv(d, log); err != nil {
			return fmt.Errorf("failed to spawn subshell: %w", err)
		}
	}

	return nil
}

func configureConfig(log *logger.Logger, components component.Component, deploySettings *deployer.Config) error {
	if deploySettings.Roxie.ClusterType == types.ClusterTypeUnknown {
		clusterType := env.GetAutoDetectedClusterType()
		log.Dimf("Detected cluster type: %v", clusterType)
		deploySettings.Roxie.ClusterType = clusterType
	}
	clusterType := deploySettings.Roxie.ClusterType
	centralDeployLocally := components.IncludesCentral() && clusterType.IsLocal()
	sensorDeployLocally := components.IncludesSensor() && clusterType.IsLocal()

	defaults, err := clusterdefaults.ApplyClusterDefaults(deploySettings)
	if err != nil {
		return err
	}
	if verbose {
		log.Dimf("Applying the following defaults based on cluster type %v:", clusterType)
		helpers.LogMultilineYaml(log, defaults)
	}

	// Deal with the "auto" resourceProfile.
	if deploySettings.Central.ResourceProfile == types.ResourceProfileAuto {
		profile := clusterdefaults.ResolveAutoResourceProfile(clusterType)
		log.Dimf("Selecting resource profile %v for Central", profile)
		deploySettings.Central.ResourceProfile = profile
	}
	if centralDeployLocally && deploySettings.Central.ResourceProfile == types.ResourceProfileAcsDefaults {
		log.Warning("You are deploying Central to a local cluster, it is recommended to specify a resource profile (or --resources=auto)")
	}

	if deploySettings.SecuredCluster.ResourceProfile == types.ResourceProfileAuto {
		profile := clusterdefaults.ResolveAutoResourceProfile(clusterType)
		log.Dimf("Selecting resource profile %v for SecuredCluster", profile)
		deploySettings.SecuredCluster.ResourceProfile = profile
	}
	if sensorDeployLocally && deploySettings.SecuredCluster.ResourceProfile == types.ResourceProfileAcsDefaults {
		log.Warning("You are deploying SecuredCluster to a local cluster, it is recommended to specify a resource profile (or --resources=auto)")
	}

	// We need to do this regardless of whether the operator is deployed or not, because
	// this includes the transformation of StackRox main image tags to semver compatible versions,
	// which we will make use of later for checking version constraints.
	if err := deploySettings.Operator.Configure(&deploySettings.Roxie); err != nil {
		return fmt.Errorf("configuring operator configuration: %w", err)
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
	if verbose {
		log.Dim("Deployment configuration:")
		helpers.LogMultilineYaml(log, deploySettings)
	}

	if !deploySettings.Central.PortForwardingSet() && !deploySettings.Central.ExposureEnabled() {
		log.Info("Enabling port-forwarding due to no exposure")
		deploySettings.Central.PortForwarding = new(true)
	}

	return nil
}

func deployValidate(components component.Component, deploySettings *deployer.Config) error {
	if components.IncludesCentral() && os.Getenv("ROXIE_SHELL") != "" {
		return errors.New("already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again")
	}

	if components.IncludesCentral() && !env.RunningInteractively && envrc == "" {
		return errors.New("running without a controlling terminal requires --envrc to be set")
	}

	clusterType := deploySettings.Roxie.ClusterType

	if env.RunningInRoxieContainer {
		// For running containerized we have specific requirements.
		if deploySettings.Central.PortForwardingEnabled() {
			return errors.New("containerized mode does not support port-forwarding")
		}
		if !deploySettings.Central.ExposureEnabled() {
			return errors.New("containerized mode requires Central exposure")
		}

		// On infra OpenShift we already get image pull secrets for Quay automatically.
		if clusterType.NeedsPullSecrets() {
			if os.Getenv("REGISTRY_USERNAME") == "" || os.Getenv("REGISTRY_PASSWORD") == "" {
				return fmt.Errorf("containerized mode requires REGISTRY_USERNAME and REGISTRY_PASSWORD environment variables for clusters of type %s", clusterType)
			}
			if _, err := os.Stat("/kubeconfig"); err != nil {
				return fmt.Errorf("containerized mode requires /kubeconfig file: %w", err)
			}
		}
	}

	if deploySettings.Operator.SkipDeploymentEnabled() && deploySettings.Operator.DeployViaOlmEnabled() {
		return errors.New("skipping operator deployment while also requesting deploying via OLM at the same time does not make sense")
	}

	if deploySettings.Roxie.KonfluxImagesEnabled() {
		if deploySettings.Operator.DeployViaOlmEnabled() {
			return errors.New("using Konflux images while deploying operator via OLM is not supported")
		}
		if !clusterType.IsOpenShift() {
			return fmt.Errorf("--konflux flag is only supported on OpenShift 4 clusters (current cluster type: %s)", clusterType)
		}
	}

	return nil
}
