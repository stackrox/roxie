package deployer

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/k8s"
	"github.com/stackrox/roxie/internal/ocihelper"
)

const (
	adminPasswordSecretName        = "admin-password"
	operatorNamespace              = "rhacs-operator-system"
	operatorDeploymentName         = "rhacs-operator-controller-manager"
	operatorBundleImageRepo        = "quay.io/rhacs-eng/stackrox-operator-bundle"
	operatorBundleImageReleaseRepo = "quay.io/rhacs-eng/release-operator-bundle"
)

var requiredCRDs = []string{
	"centrals.platform.stackrox.io",
	"securedclusters.platform.stackrox.io",
	"securitypolicies.config.stackrox.io",
}

// deployOperatorNonOLM deploys the RHACS operator without OLM
func (d *Deployer) deployOperatorNonOLM(ctx context.Context) error {
	d.logger.Infof("Operator tag: %s", d.config.Operator.Version)
	if d.config.Roxie.KonfluxImages {
		if err := d.ensureKonfluxImageRewriting(ctx); err != nil {
			return fmt.Errorf("failed to configure Konflux image rewriting: %w", err)
		}
	} else {
		if err := d.removeKonfluxImageRewriting(ctx); err != nil {
			return fmt.Errorf("failed to remove Konflux ImageContentSourcePolicy: %v", err)
		}
	}
	bundleImage := d.getOperatorBundleImage()

	bundleDir, err := d.downloadAndExtractOperatorBundle(ctx, bundleImage)
	if err != nil {
		return fmt.Errorf("failed to download operator bundle: %w", err)
	}
	defer d.cleanupTempDir(bundleDir, "operator bundle directory")

	d.logger.Infof("Bundle image: %s", bundleImage)

	crdFiles, err := d.identifyCRDFileNames(bundleDir)
	if err != nil {
		return err
	}

	if err := d.applyCRDsToCluster(ctx, crdFiles); err != nil {
		return err
	}

	if err := d.deployOperatorFromCSV(ctx, bundleDir); err != nil {
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
	if err := ocihelper.ExtractManifestsFromImage(ctx, d.logger, bundleImage, bundleDir); err != nil {
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
		bundleImage := d.getOperatorBundleImage()
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

func (d *Deployer) getOperatorBundleImage() string {
	if d.config.Roxie.KonfluxImages {
		d.logger.Infof("Using Konflux-built operator bundle image")
		return fmt.Sprintf(operatorBundleImageReleaseRepo+":v%s", d.config.Operator.Version)
	}
	return fmt.Sprintf(operatorBundleImageRepo+":v%s", d.config.Operator.Version)
}

// ensureKonfluxImageRewriting configures image rewriting for Konflux images
func (d *Deployer) ensureKonfluxImageRewriting(ctx context.Context) error {
	if !env.GetCurrentClusterType().IsOpenShift() {
		return errors.New("image rewriting for Konflux is only supported on OpenShift4 clusters")
	}

	d.logger.Info("Configuring ImageContentSourcePolicy for Konflux images on OpenShift4...")
	return d.applyImageContentSourcePolicy(ctx)
}

// applyImageContentSourcePolicy creates the ImageContentSourcePolicy for Konflux image mirrors
func (d *Deployer) applyImageContentSourcePolicy(ctx context.Context) error {
	// Define repository digest mirrors as Go data structures
	rewrite := func(from, to string) map[string]interface{} {
		source := fmt.Sprintf("registry.redhat.io/advanced-cluster-security/%s", from)
		mirror := fmt.Sprintf("quay.io/rhacs-eng/%s", to)
		if d.verbose {
			d.logger.Dimf("Image rewriting rule: %s -> %s", source, mirror)
		}
		return map[string]interface{}{
			"source":  source,
			"mirrors": []string{mirror},
		}
	}
	repositoryDigestMirrors := []map[string]interface{}{
		rewrite("rhacs-operator-bundle", "release-operator-bundle"),
		rewrite("rhacs-rhel8-operator", "release-operator"),
		rewrite("rhacs-main-rhel8", "release-main"),
		rewrite("rhacs-scanner-rhel8", "release-scanner"),
		rewrite("rhacs-scanner-slim-rhel8", "release-scanner-slim"),
		rewrite("rhacs-scanner-db-rhel8", "release-scanner-db"),
		rewrite("rhacs-scanner-db-slim-rhel8", "release-scanner-db-slim"),
		rewrite("rhacs-collector-slim-rhel8", "release-collector-slim"),
		rewrite("rhacs-collector-rhel8", "release-collector"),
		rewrite("rhacs-fact-rhel8", "release-fact"),
		rewrite("rhacs-roxctl-rhel8", "release-roxctl"),
		rewrite("rhacs-central-db-rhel8", "release-central-db"),
		rewrite("rhacs-scanner-v4-db-rhel8", "release-scanner-v4-db"),
		rewrite("rhacs-scanner-v4-rhel8", "release-scanner-v4"),

		// Also support downstream image rewriting for upcoming UBI9/rhel9 images.
		rewrite("rhacs-operator-bundle-rhel9", "release-operator-bundle"),
		rewrite("rhacs-rhel9-operator", "release-operator"),
		rewrite("rhacs-main-rhel9", "release-main"),
		rewrite("rhacs-scanner-rhel9", "release-scanner"),
		rewrite("rhacs-scanner-slim-rhel9", "release-scanner-slim"),
		rewrite("rhacs-scanner-db-rhel9", "release-scanner-db"),
		rewrite("rhacs-scanner-db-slim-rhel9", "release-scanner-db-slim"),
		rewrite("rhacs-collector-slim-rhel9", "release-collector-slim"),
		rewrite("rhacs-collector-rhel9", "release-collector"),
		rewrite("rhacs-fact-rhel9", "release-fact"),
		rewrite("rhacs-roxctl-rhel9", "release-roxctl"),
		rewrite("rhacs-central-db-rhel9", "release-central-db"),
		rewrite("rhacs-scanner-v4-db-rhel9", "release-scanner-v4-db"),
		rewrite("rhacs-scanner-v4-rhel9", "release-scanner-v4"),
	}

	icsp := map[string]interface{}{
		"apiVersion": "operator.openshift.io/v1alpha1",
		"kind":       "ImageContentSourcePolicy",
		"metadata": map[string]interface{}{
			"name": "acs-konflux-builds",
		},
		"spec": map[string]interface{}{
			"repositoryDigestMirrors": repositoryDigestMirrors,
		},
	}

	yamlData, err := yaml.Marshal(icsp)
	if err != nil {
		return fmt.Errorf("failed to marshal ImageContentSourcePolicy: %w", err)
	}

	d.logger.Dim("Applying ImageContentSourcePolicy...")
	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to apply ImageContentSourcePolicy: %w", err)
	}

	d.logger.Successf("✓ ImageContentSourcePolicy 'acs-konflux-builds' applied")
	d.logger.Info("Note: OpenShift nodes may need to restart to apply the image mirroring configuration")

	return nil
}

// removeKonfluxImageRewriting removes the ImageContentSourcePolicy for Konflux images if it exists
func (d *Deployer) removeKonfluxImageRewriting(ctx context.Context) error {
	if !env.GetCurrentClusterType().IsOpenShift() {
		return nil
	}

	d.logger.Dim("Removing Konflux ImageContentSourcePolicy if present...")
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"delete", "imagecontentsourcepolicy", "acs-konflux-builds", "--ignore-not-found=true"},
	})
	if err != nil {
		return fmt.Errorf("failed to delete ImageContentSourcePolicy: %w", err)
	}

	return nil
}

// deployOperatorFromCSV deploys the operator from CSV
func (d *Deployer) deployOperatorFromCSV(ctx context.Context, bundleDir string) error {
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

	d.logger.Info("📋 Operator deployment plan:")
	d.logger.Dim(fmt.Sprintf("  • Namespace: %s", operatorNamespace))
	d.logger.Dim(fmt.Sprintf("  • ServiceAccount: %s", serviceAccountName))

	if err := d.createOperatorNamespace(ctx); err != nil {
		return err
	}

	if err := d.createServiceAccount(ctx, operatorNamespace, serviceAccountName); err != nil {
		return err
	}

	if err := d.createClusterRoleFromCSV(ctx, deploymentSpec); err != nil {
		return err
	}

	if err := d.createClusterRoleBinding(ctx, operatorNamespace, serviceAccountName); err != nil {
		return err
	}

	if err := d.createDeploymentFromCSV(ctx, operatorNamespace, deploymentSpec); err != nil {
		return err
	}

	// Apply bundle service resources (if they exist)
	_ = d.applyBundleServiceResources(ctx, bundleDir, operatorNamespace)

	if err := d.waitForOperatorReady(ctx, operatorNamespace, operatorDeploymentName, 300); err != nil {
		return err
	}

	d.logger.Success("🎉 Operator deployment completed successfully!")
	return nil
}

// parseCSVDeploymentSpec parses the CSV file
func (d *Deployer) parseCSVDeploymentSpec(csvFile string) (map[string]interface{}, error) {
	content, err := os.ReadFile(csvFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV file: %w", err)
	}

	var csvContent map[string]interface{}
	if err := yaml.Unmarshal(content, &csvContent); err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	spec := csvContent["spec"].(map[string]interface{})
	installSpec := spec["install"].(map[string]interface{})["spec"].(map[string]interface{})

	deployments := installSpec["deployments"].([]interface{})
	clusterPermissions := installSpec["clusterPermissions"].([]interface{})

	metadata := csvContent["metadata"].(map[string]interface{})

	deploymentSpec := map[string]interface{}{
		"name":                metadata["name"],
		"deployments":         deployments,
		"cluster_permissions": clusterPermissions,
		"service_account":     "rhacs-operator-controller-manager",
	}

	if len(clusterPermissions) > 0 {
		firstPerm := clusterPermissions[0].(map[string]interface{})
		if sa, ok := firstPerm["serviceAccountName"]; ok {
			deploymentSpec["service_account"] = sa
		}
	}

	return deploymentSpec, nil
}

// createOperatorNamespace creates the operator namespace
func (d *Deployer) createOperatorNamespace(ctx context.Context) error {
	nsYAML := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    name: %s
    app.kubernetes.io/managed-by: roxie
`, operatorNamespace, operatorNamespace)

	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: strings.NewReader(nsYAML),
	})
	return err
}

// createServiceAccount creates a service account
func (d *Deployer) createServiceAccount(ctx context.Context, namespace, name string) error {
	sa := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]string{"app": "rhacs-operator"},
		},
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

// createClusterRoleFromCSV creates ClusterRole from CSV
func (d *Deployer) createClusterRoleFromCSV(ctx context.Context, deploymentSpec map[string]interface{}) error {
	clusterPermissions := deploymentSpec["cluster_permissions"].([]interface{})
	if len(clusterPermissions) == 0 {
		d.logger.Warning("No cluster permissions found in CSV")
		return nil
	}

	firstPerm := clusterPermissions[0].(map[string]interface{})
	rules := firstPerm["rules"]

	clusterRole := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRole",
		"metadata": map[string]interface{}{
			"name":   "rhacs-operator-manager-role",
			"labels": map[string]string{"app": "rhacs-operator"},
		},
		"rules": rules,
	}

	yamlData, err := yaml.Marshal(clusterRole)
	if err != nil {
		return fmt.Errorf("failed to marshal ClusterRole 'rhacs-operator-manager-role': %w", err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create ClusterRole 'rhacs-operator-manager-role': %w", err)
	}
	return nil
}

// createClusterRoleBinding creates ClusterRoleBinding
func (d *Deployer) createClusterRoleBinding(ctx context.Context, namespace, serviceAccountName string) error {
	crb := map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "ClusterRoleBinding",
		"metadata": map[string]interface{}{
			"name":   "rhacs-operator-manager-rolebinding",
			"labels": map[string]string{"app": "rhacs-operator"},
		},
		"roleRef": map[string]interface{}{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "ClusterRole",
			"name":     "rhacs-operator-manager-role",
		},
		"subjects": []interface{}{
			map[string]interface{}{
				"kind":      "ServiceAccount",
				"name":      serviceAccountName,
				"namespace": namespace,
			},
		},
	}

	yamlData, err := yaml.Marshal(crb)
	if err != nil {
		return fmt.Errorf("failed to marshal ClusterRoleBinding 'rhacs-operator-manager-rolebinding' for ServiceAccount '%s/%s': %w", namespace, serviceAccountName, err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create ClusterRoleBinding 'rhacs-operator-manager-rolebinding': %w", err)
	}
	return nil
}

// createDeploymentFromCSV creates Deployment from CSV
func (d *Deployer) createDeploymentFromCSV(ctx context.Context, namespace string, deploymentSpec map[string]interface{}) error {
	deployments := deploymentSpec["deployments"].([]interface{})
	if len(deployments) == 0 {
		return errors.New("no deployments found in CSV")
	}

	csvDeployment := deployments[0].(map[string]interface{})
	deploymentName, _ := csvDeployment["name"].(string)
	if deploymentName == "" {
		deploymentName = operatorDeploymentName
	}

	deploymentTemplate := csvDeployment["spec"].(map[string]interface{})

	deployment := map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":      deploymentName,
			"namespace": namespace,
			"labels":    csvDeployment["label"],
		},
		"spec": deploymentTemplate,
	}

	spec := deployment["spec"].(map[string]interface{})
	if template, ok := spec["template"].(map[string]interface{}); ok {
		if podSpec, ok := template["spec"].(map[string]interface{}); ok {
			podSpec["serviceAccountName"] = deploymentSpec["service_account"]
		}
	}

	yamlData, err := yaml.Marshal(deployment)
	if err != nil {
		return fmt.Errorf("failed to marshal Deployment '%s/%s': %w", namespace, deploymentName, err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create Deployment '%s/%s': %w", namespace, deploymentName, err)
	}
	return nil
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

func generateClusterName() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(9000))
	return fmt.Sprintf("sensor-%d", n.Int64()+1000)
}

// teardownOperatorNonOLM removes the operator when installed without OLM.
func (d *Deployer) teardownOperatorNonOLM(ctx context.Context) error {
	d.logger.Info("🧹 Tearing down operator deployed without OLM...")

	// Delete operator namespace.
	d.runKubectl(ctx, k8s.KubectlOptions{
		Args:         []string{"delete", "namespace", operatorNamespace, "--wait=false"},
		IgnoreErrors: true,
	})

	// Delete cluster-scoped resources created by non-OLM flow.
	clusterResources := []struct {
		name string
		kind string
	}{
		{name: "rhacs-operator-manager-rolebinding", kind: "clusterrolebinding"},
		{name: "rhacs-operator-manager-role", kind: "clusterrole"},
	}
	for _, resource := range clusterResources {
		d.runKubectl(ctx, k8s.KubectlOptions{
			Args:         []string{"delete", resource.kind, resource.name, "--ignore-not-found=true"},
			IgnoreErrors: true,
		})
	}

	if err := d.waitForNamespaceDeletion(operatorNamespace); err != nil {
		d.logger.Warningf("Namespace %s deletion incomplete: %v", operatorNamespace, err)
	}

	d.logger.Success("✓ Non-OLM operator resources removed")
	return nil
}

// teardownOperator removes the operator if it exists, detecting the deployment mode automatically.
func (d *Deployer) teardownOperator(ctx context.Context) error {
	operatorExists, operatorMode, err := d.detectOperatorDeploymentMode(ctx)
	if err != nil {
		return fmt.Errorf("detecting operator deployment mode: %w", err)
	}
	if !operatorExists {
		d.logger.Dim("No operator deployment found, skipping operator teardown")
		return nil
	}

	if operatorMode == OperatorModeOLM {
		return d.teardownOperatorOLM(ctx)
	}
	return d.teardownOperatorNonOLM(ctx)
}
