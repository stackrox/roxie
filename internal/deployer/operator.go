package deployer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/stackrox/roxie/internal/constants"
	"github.com/stackrox/roxie/internal/k8s"
	"github.com/stackrox/roxie/internal/ocihelper"
)

const (
	adminPasswordSecretName = "admin-password"
	// operatorNamespace is the single-operator namespace used by the OLM path
	// and other code that still references the package-level constant.
	operatorNamespace      = operatorNamespaceSystem
	operatorDeploymentName = "rhacs-operator-controller-manager"
	managerContainerName   = "manager"
)

var requiredCRDs = []string{
	"centrals.platform.stackrox.io",
	"securedclusters.platform.stackrox.io",
	"securitypolicies.config.stackrox.io",
}

// deployOperatorNonOLM deploys one RHACS operator instance without OLM.
func (d *Deployer) deployOperatorNonOLM(ctx context.Context, instance OperatorInstanceConfig) error {
	d.logger.Infof("Operator tag: %s (namespace %s)", instance.Version, instance.Namespace)
	bundleImage, err := d.resolveBundleImage(ctx, instance)
	if err != nil {
		return fmt.Errorf("resolving operator bundle image: %w", err)
	}

	bundleDir, err := d.downloadAndExtractOperatorBundle(ctx, bundleImage)
	if err != nil {
		return fmt.Errorf("failed to download operator bundle: %w", err)
	}
	defer d.cleanupTempDir(bundleDir, "operator bundle directory")

	d.logger.Infof("Bundle image: %s", bundleImage)

	// Only the newest planned operator version may apply CRDs, so an older
	// companion operator cannot downgrade cluster CRD schemas.
	operatorTag := instance.Version.ToOperatorTag()
	if operatorTag == d.config.NewestOperatorVersion() {
		crdFiles, err := d.identifyCRDFileNames(bundleDir)
		if err != nil {
			return err
		}
		if err := d.applyCRDsToCluster(ctx, crdFiles); err != nil {
			return err
		}
	} else {
		d.logger.Dimf("Skipping CRD apply for older operator version %s (newest is %s)",
			operatorTag, d.config.NewestOperatorVersion())
	}

	if err := d.deployOperatorFromCSV(ctx, bundleDir, instance); err != nil {
		return err
	}

	return nil
}

// downloadAndExtractOperatorBundle downloads and extracts the operator bundle
func (d *Deployer) downloadAndExtractOperatorBundle(ctx context.Context, bundleImage string) (string, error) {
	bundleDir, err := os.MkdirTemp(d.tempDir, "stackrox-operator-bundle-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	d.logger.Dimf("Created temporary directory: %s", bundleDir)
	d.logger.Info("Pulling and extracting operator bundle image...")

	// The bundle images only contain platform-agnostic YAML files.
	if err := ocihelper.ExtractManifestsFromImage(ctx, d.logger, bundleImage, bundleDir, d.containerRuntimeSocket); err != nil {
		os.RemoveAll(bundleDir)
		return "", fmt.Errorf("failed to copy bundle contents: %w", err)
	}

	d.logger.Successf("✓ Bundle extracted to: %s", bundleDir)
	return bundleDir, nil
}

// identifyCRDFileNames identifies CRD files in the bundle directory.
// Returns list of CRD files found in the bundle.
func (d *Deployer) identifyCRDFileNames(bundleDir string) ([]string, error) {
	var crdFiles []string

	err := filepath.Walk(bundleDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			d.logger.Warningf("Failed to read file %q from extracted bundle: %v", path, err)
			return nil
		}

		var meta struct {
			Kind string `yaml:"kind"`
		}
		if err := yaml.Unmarshal(content, &meta); err != nil {
			d.logger.Warningf("Failed to unmarshal file %q from extracted bundle: %v", path, err)
			return nil
		}

		if meta.Kind == "CustomResourceDefinition" {
			crdFiles = append(crdFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk bundle directory: %w", err)
	}

	return crdFiles, nil
}

// applyCRDsToCluster applies CRD files to the cluster
func (d *Deployer) applyCRDsToCluster(ctx context.Context, crdFiles []string) error {
	d.logger.Infof("Applying %d CRD(s) to cluster", len(crdFiles))

	for _, crdFile := range crdFiles {
		result, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"apply", "-f", crdFile},
		})
		if err != nil {
			d.logger.Errorf("kubectl stderr: %s", result.Stderr)
			return fmt.Errorf("failed to apply CRD %s: %w\nStderr: %s", crdFile, err, result.Stderr)
		}

		basename := filepath.Base(crdFile)
		d.logger.Successf("✓ Successfully applied CRD %s", basename)
	}

	return nil
}

// ensureCRDsInstalled ensures required CRDs exist
func (d *Deployer) ensureCRDsInstalled(ctx context.Context) error {
	var missing []string
	for _, crd := range requiredCRDs {
		_, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "crd", crd},
		})
		if err != nil {
			missing = append(missing, crd)
		}
	}

	if len(missing) > 0 {
		crdInstance := d.config.NewestOperatorInstance()
		bundleImage, err := d.resolveBundleImage(ctx, crdInstance)
		if err != nil {
			return fmt.Errorf("resolving operator bundle image: %w", err)
		}
		d.logger.Warningf("Missing CRDs detected (%s)", strings.Join(missing, ", "))
		d.logger.Warningf("Fetching bundle %s", bundleImage)

		bundleDir, err := d.downloadAndExtractOperatorBundle(ctx, bundleImage)
		if err != nil {
			return err
		}
		defer d.cleanupTempDir(bundleDir, "CRD bundle directory")

		crdFiles, err := d.identifyCRDFileNames(bundleDir)
		if err != nil {
			return err
		}

		return d.applyCRDsToCluster(ctx, crdFiles)
	}

	return nil
}

// resolveBundleImage returns the operator bundle image to use for the given instance, probing
// the configured registry first and falling back to constants.DefaultRegistry if the bundle
// does not exist there.
//
// This is done because upstream StackRox builds (quay.io/stackrox-io) do not publish operator bundles.
func (d *Deployer) resolveBundleImage(ctx context.Context, instance OperatorInstanceConfig) (string, error) {
	bundleImage := instance.BundleImage()
	if instance.ImageRegistry == constants.DefaultRegistry {
		return bundleImage, nil
	}

	if err := ocihelper.VerifyImageExistence(ctx, d.logger, bundleImage); err != nil {
		var te *transport.Error
		if errors.As(err, &te) && te.StatusCode == http.StatusNotFound {
			fallback := instance
			fallback.ImageRegistry = constants.DefaultRegistry
			fallbackImage := fallback.BundleImage()
			d.logger.Warningf("No operator bundle found at %s, falling back to %s", bundleImage, fallbackImage)
			return fallbackImage, nil
		}
		return "", fmt.Errorf("verifying existence of operator bundle image %s: %w", bundleImage, err)
	}

	return bundleImage, nil
}

// needsOperatorPullSecrets reports whether a pull secret should be created for the
// operator's own deployment namespace.
func (d *Deployer) needsOperatorPullSecrets(instance OperatorInstanceConfig) bool {
	if d.config.Roxie.UsesCustomRegistry() {
		return d.NeedsPullSecrets()
	}
	return instance.KonfluxImagesEnabled() && d.config.Roxie.ClusterType.NeedsDefaultRegistryPullSecrets()
}

// deployOperatorFromCSV deploys the operator from CSV into the given instance namespace.
func (d *Deployer) deployOperatorFromCSV(ctx context.Context, bundleDir string, instance OperatorInstanceConfig) error {
	csvFile := filepath.Join(bundleDir, "rhacs-operator.clusterserviceversion.yaml")
	if _, err := os.Stat(csvFile); os.IsNotExist(err) {
		return errors.New("ClusterServiceVersion file not found in bundle")
	}

	d.logger.Info("🔍 Parsing ClusterServiceVersion deployment specification")

	deploymentSpec, err := d.parseCSVDeploymentSpec(csvFile)
	if err != nil {
		return err
	}

	serviceAccountName := deploymentSpec["service_account"].(string)
	d.useOperatorPullSecrets = d.needsOperatorPullSecrets(instance)

	d.logger.Info("📋 Operator deployment plan:")
	d.logger.Dimf("  • Namespace: %s", instance.Namespace)
	d.logger.Dimf("  • ServiceAccount: %s", serviceAccountName)
	d.logger.Dimf("  • Setting up pull secrets: %v", d.useOperatorPullSecrets)
	if d.verbose && len(instance.EnvVars) > 0 {
		d.logger.Dimf("  • Custom operator env vars: %d", len(instance.EnvVars))
		for _, envVar := range envVarsToSortedList(instance.EnvVars) {
			ev := envVar.(map[string]any)
			d.logger.Dimf("    %s=%s", ev["name"], ev["value"])
		}
	}

	if err := d.prepareNamespace(ctx, instance.Namespace, d.useOperatorPullSecrets); err != nil {
		return err
	}

	if err := d.createServiceAccount(ctx, instance.Namespace, serviceAccountName); err != nil {
		return err
	}

	if err := d.createClusterRoleFromCSV(ctx, deploymentSpec, instance); err != nil {
		return err
	}

	if err := d.createClusterRoleBinding(ctx, instance, serviceAccountName); err != nil {
		return err
	}

	if err := d.createDeploymentFromCSV(ctx, instance, deploymentSpec); err != nil {
		return err
	}

	// Apply bundle service resources (if they exist)
	_ = d.applyBundleServiceResources(ctx, bundleDir, instance.Namespace)

	if err := d.waitForOperatorReady(ctx, instance.Namespace, operatorDeploymentName, 300); err != nil {
		return err
	}

	d.logger.Successf("🎉 Operator deployment completed successfully in %s!", instance.Namespace)
	return nil
}

// parseCSVDeploymentSpec parses the CSV file
func (d *Deployer) parseCSVDeploymentSpec(csvFile string) (map[string]any, error) {
	content, err := os.ReadFile(csvFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV file: %w", err)
	}

	var csvContent map[string]any
	if err := yaml.Unmarshal(content, &csvContent); err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	spec := csvContent["spec"].(map[string]any)
	installSpec := spec["install"].(map[string]any)["spec"].(map[string]any)

	deployments := installSpec["deployments"].([]any)
	clusterPermissions := installSpec["clusterPermissions"].([]any)

	metadata := csvContent["metadata"].(map[string]any)

	deploymentSpec := map[string]any{
		"name":                metadata["name"],
		"deployments":         deployments,
		"cluster_permissions": clusterPermissions,
		"service_account":     "rhacs-operator-controller-manager",
	}

	if len(clusterPermissions) > 0 {
		firstPerm := clusterPermissions[0].(map[string]any)
		if sa, ok := firstPerm["serviceAccountName"]; ok {
			deploymentSpec["service_account"] = sa
		}
	}

	return deploymentSpec, nil
}

// createServiceAccount creates a service account
func (d *Deployer) createServiceAccount(ctx context.Context, namespace, name string) error {
	sa := map[string]any{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]string{"app": "rhacs-operator"},
		},
	}

	if d.useOperatorPullSecrets {
		sa["imagePullSecrets"] = []map[string]string{
			{"name": "stackrox"},
		}
	}

	yamlData, err := yaml.Marshal(sa)
	if err != nil {
		return fmt.Errorf("failed to marshal ServiceAccount '%s/%s': %w", namespace, name, err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create ServiceAccount '%s/%s': %w", namespace, name, err)
	}
	return nil
}

// createClusterRoleFromCSV creates ClusterRole from CSV for the given operator instance.
func (d *Deployer) createClusterRoleFromCSV(ctx context.Context, deploymentSpec map[string]any, instance OperatorInstanceConfig) error {
	clusterPermissions := deploymentSpec["cluster_permissions"].([]any)
	if len(clusterPermissions) == 0 {
		d.logger.Warning("No cluster permissions found in CSV")
		return nil
	}

	firstPerm := clusterPermissions[0].(map[string]any)
	rules := firstPerm["rules"]
	roleName := instance.ClusterRoleName()

	clusterRole := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]any{
			"name":   roleName,
			"labels": map[string]string{"app": "rhacs-operator"},
		},
		"rules": rules,
	}

	yamlData, err := yaml.Marshal(clusterRole)
	if err != nil {
		return fmt.Errorf("failed to marshal ClusterRole '%s': %w", roleName, err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create ClusterRole '%s': %w", roleName, err)
	}
	return nil
}

// createClusterRoleBinding creates ClusterRoleBinding for the given operator instance.
func (d *Deployer) createClusterRoleBinding(ctx context.Context, instance OperatorInstanceConfig, serviceAccountName string) error {
	roleName := instance.ClusterRoleName()
	bindingName := instance.ClusterRoleBindingName()

	crb := map[string]any{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata": map[string]any{
			"name":   bindingName,
			"labels": map[string]string{"app": "rhacs-operator"},
		},
		"roleRef": map[string]any{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole",
			"name":     roleName,
		},
		"subjects": []any{
			map[string]any{
				"kind":      "ServiceAccount",
				"name":      serviceAccountName,
				"namespace": instance.Namespace,
			},
		},
	}

	yamlData, err := yaml.Marshal(crb)
	if err != nil {
		return fmt.Errorf("failed to marshal ClusterRoleBinding '%s' for ServiceAccount '%s/%s': %w", bindingName, instance.Namespace, serviceAccountName, err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create ClusterRoleBinding '%s': %w", bindingName, err)
	}
	return nil
}

// createDeploymentFromCSV creates Deployment from CSV for the given operator instance.
func (d *Deployer) createDeploymentFromCSV(ctx context.Context, instance OperatorInstanceConfig, deploymentSpec map[string]any) error {
	deployments := deploymentSpec["deployments"].([]any)
	if len(deployments) == 0 {
		return errors.New("no deployments found in CSV")
	}

	csvDeployment := deployments[0].(map[string]any)
	deploymentName, _ := csvDeployment["name"].(string)
	if deploymentName == "" {
		deploymentName = operatorDeploymentName
	}

	deploymentTemplate := csvDeployment["spec"].(map[string]any)

	deployment := map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      deploymentName,
			"namespace": instance.Namespace,
			"labels":    csvDeployment["label"],
		},
		"spec": deploymentTemplate,
	}

	podSpecAny, found, err := unstructured.NestedFieldNoCopy(deployment, "spec", "template", "spec")
	if err != nil {
		return fmt.Errorf("extracting pod spec from operator deployment object: %w", err)
	}
	if !found {
		return errors.New("missing pod spec in deployment object")
	}
	podSpec, ok := podSpecAny.(map[string]any)
	if !ok {
		return fmt.Errorf("pod spec in deployment object of invalid type %T", podSpecAny)
	}

	managerContainer, err := managerContainerFromPodSpec(podSpec)
	if err != nil {
		return fmt.Errorf("extracting manager container from operator pod spec: %w", err)
	}

	operatorImage := instance.OperatorImage()
	podSpec["serviceAccountName"] = deploymentSpec["service_account"]
	if current, _ := managerContainer["image"].(string); current != operatorImage {
		d.logger.Infof("Rewriting operator image to %s", operatorImage)
		managerContainer["image"] = operatorImage
	}

	if len(instance.EnvVars) > 0 {
		injectEnvVarsIntoManagerContainer(managerContainer, instance.EnvVars)
	}

	yamlData, err := yaml.Marshal(deployment)
	if err != nil {
		return fmt.Errorf("failed to marshal Deployment '%s/%s': %w", instance.Namespace, deploymentName, err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create Deployment '%s/%s': %w", instance.Namespace, deploymentName, err)
	}
	return nil
}

func managerContainerFromPodSpec(podSpec map[string]any) (map[string]any, error) {
	containers, ok := podSpec["containers"].([]any)
	if !ok {
		return nil, errors.New("no containers found in deployment pod spec")
	}

	for _, c := range containers {
		container, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if container["name"] == managerContainerName {
			return container, nil
		}
	}
	return nil, fmt.Errorf("container %q missing from operator pod spec", managerContainerName)
}

// injectEnvVarsIntoManagerContainer merges configured operator env vars into
// the manager container, overriding any existing env vars with the same name.
func injectEnvVarsIntoManagerContainer(container map[string]any, envVars map[string]string) {
	existing := make(map[string]int)
	envList, _ := container["env"].([]any)
	for i, item := range envList {
		if envVar, ok := item.(map[string]any); ok {
			if name, ok := envVar["name"].(string); ok {
				existing[name] = i
			}
		}
	}

	for _, envVar := range envVarsToSortedList(envVars) {
		name := envVar.(map[string]any)["name"].(string)
		if idx, found := existing[name]; found {
			envList[idx] = envVar
		} else {
			envList = append(envList, envVar)
		}
	}

	container["env"] = envList
}

func (d *Deployer) applyBundleServiceResources(ctx context.Context, bundleDir, namespace string) error {
	serviceFile := filepath.Join(bundleDir, "rhacs-operator-controller-manager-metrics-service_v1_service.yaml")
	if _, err := os.Stat(serviceFile); err == nil {
		d.runKubectl(ctx, k8s.KubectlOptions{
			Args:         []string{"apply", "-n", namespace, "-f", serviceFile},
			IgnoreErrors: true,
		})
	}

	clusterRoleFile := filepath.Join(bundleDir, "rhacs-operator-metrics-reader_rbac.authorization.k8s.io_v1_clusterrole.yaml")
	if _, err := os.Stat(clusterRoleFile); err == nil {
		d.runKubectl(ctx, k8s.KubectlOptions{
			Args:         []string{"apply", "-f", clusterRoleFile},
			IgnoreErrors: true,
		})
	}

	return nil
}

// waitForOperatorReady waits for operator deployment to be ready
func (d *Deployer) waitForOperatorReady(ctx context.Context, namespace, deploymentName string, timeout int) error {
	d.logger.Info("⏳ Waiting for operator deployment to become ready...")

	start := time.Now()
	for time.Since(start) < time.Duration(timeout)*time.Second {
		result, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "deployment", deploymentName, "-n", namespace, "-o", "jsonpath={.status.readyReplicas}"},
		})
		if err == nil && result.Stdout != "" {
			replicas := strings.TrimSpace(result.Stdout)
			if replicas != "0" && replicas != "" {
				d.logger.Successf("✓ Operator deployment is ready (%s replicas)", replicas)
				return nil
			}
		}

		time.Sleep(2 * time.Second)
	}

	return errors.New("timeout waiting for operator deployment to become ready")
}

// teardownOperatorNonOLMInNamespace removes a non-OLM operator from the given namespace
// and deletes its cluster-scoped RBAC resources for that instance.
func (d *Deployer) teardownOperatorNonOLMInNamespace(ctx context.Context, instance OperatorInstanceConfig) error {
	d.logger.Infof("🧹 Tearing down non-OLM operator in namespace %s...", instance.Namespace)

	d.runKubectl(ctx, k8s.KubectlOptions{
		Args:         []string{"delete", "namespace", instance.Namespace, "--wait=false"},
		IgnoreErrors: true,
	})

	for _, resource := range []struct {
		name string
		kind string
	}{
		{name: instance.ClusterRoleBindingName(), kind: "clusterrolebinding"},
		{name: instance.ClusterRoleName(), kind: "clusterrole"},
	} {
		d.runKubectl(ctx, k8s.KubectlOptions{
			Args:         []string{"delete", resource.kind, resource.name, "--ignore-not-found=true"},
			IgnoreErrors: true,
		})
	}

	if err := d.waitForNamespaceDeletion(instance.Namespace); err != nil {
		d.logger.Warningf("Namespace %s deletion incomplete: %v", instance.Namespace, err)
	}

	d.logger.Successf("✓ Non-OLM operator resources removed from %s", instance.Namespace)
	return nil
}

// teardownAllOperatorClusterRBAC deletes all known operator ClusterRole/Binding names.
func (d *Deployer) teardownAllOperatorClusterRBAC(ctx context.Context) {
	for _, instance := range []OperatorInstanceConfig{
		{Namespace: operatorNamespaceSystem},
		{Namespace: operatorNamespaceCentral, RoleNameSuffix: roleNameSuffixCentral},
		{Namespace: operatorNamespaceSensor, RoleNameSuffix: roleNameSuffixSensor},
	} {
		d.runKubectl(ctx, k8s.KubectlOptions{
			Args:         []string{"delete", "clusterrolebinding", instance.ClusterRoleBindingName(), "--ignore-not-found=true"},
			IgnoreErrors: true,
		})
		d.runKubectl(ctx, k8s.KubectlOptions{
			Args:         []string{"delete", "clusterrole", instance.ClusterRoleName(), "--ignore-not-found=true"},
			IgnoreErrors: true,
		})
	}
}

// teardownOperatorNonOLM removes non-OLM operators from all known namespaces.
func (d *Deployer) teardownOperatorNonOLM(ctx context.Context) error {
	d.logger.Info("🧹 Tearing down operator deployed without OLM...")

	for _, ns := range AllOperatorNamespaces {
		if !d.namespaceExists(ns) {
			continue
		}
		instance := OperatorInstanceConfig{Namespace: ns}
		switch ns {
		case operatorNamespaceCentral:
			instance.RoleNameSuffix = roleNameSuffixCentral
		case operatorNamespaceSensor:
			instance.RoleNameSuffix = roleNameSuffixSensor
		}
		_ = d.teardownOperatorNonOLMInNamespace(ctx, instance)
	}

	d.teardownAllOperatorClusterRBAC(ctx)
	d.logger.Success("✓ Non-OLM operator resources removed")
	return nil
}

// teardownOperator removes the operator if it exists, detecting the deployment mode automatically.
func (d *Deployer) teardownOperator(ctx context.Context) error {
	operatorExists, operatorMode, err := d.detectOperatorDeploymentMode(ctx)
	if err != nil {
		return fmt.Errorf("detecting operator deployment mode: %w", err)
	}
	if operatorExists && operatorMode == OperatorModeOLM {
		return d.teardownOperatorOLM(ctx)
	}

	foundAny := operatorExists
	for _, ns := range AllOperatorNamespaces {
		if ns == operatorNamespaceSystem {
			continue // already covered by detectOperatorDeploymentMode for non-OLM
		}
		if d.operatorDeploymentExists(ctx, ns) {
			foundAny = true
			break
		}
	}
	if !foundAny {
		d.logger.Dim("No operator deployment found, skipping operator teardown")
		return nil
	}

	return d.teardownOperatorNonOLM(ctx)
}

func (d *Deployer) operatorDeploymentExists(ctx context.Context, namespace string) bool {
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "deployment", operatorDeploymentName, "-n", namespace},
	})
	return err == nil
}
