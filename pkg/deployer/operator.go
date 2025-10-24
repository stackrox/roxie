package deployer

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/stackrox/roxie/pkg/helpers"
)

const (
	adminPasswordSecretName = "admin-password"
	operatorNamespace       = "rhacs-operator-system"
	operatorDeploymentName  = "rhacs-operator-controller-manager"
)

// deployOperator deploys the RHACS operator
func (d *Deployer) deployOperator(ctx context.Context) error {
	d.logger.Infof("Operator tag: %s", d.operatorTag)
	bundleImage := d.getOperatorBundleImage()

	bundleDir, err := d.downloadAndExtractOperatorBundle(ctx, bundleImage)
	if err != nil {
		return fmt.Errorf("failed to download operator bundle: %w", err)
	}
	defer d.cleanupTempDir(bundleDir, "operator bundle directory")

	d.logger.Infof("Bundle image: %s", bundleImage)

	crdFiles, err := d.identifyCRDFiles(bundleDir)
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
	bundleDir, err := os.MkdirTemp("", "stackrox-operator-bundle-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	d.logger.Dim(fmt.Sprintf("Created temporary directory: %s", bundleDir))

	containerTool := helpers.GetContainerTool()
	d.logger.Dim(fmt.Sprintf("Using %s to extract bundle", containerTool))

	// Check if image exists locally
	inspectCmd := exec.CommandContext(ctx, containerTool, "inspect", bundleImage)
	if err := inspectCmd.Run(); err != nil {
		// Image doesn't exist locally, pull it
		d.logger.Info("Pulling operator bundle image...")
		pullCmd := exec.CommandContext(ctx, containerTool, "pull", bundleImage)
		if output, err := pullCmd.CombinedOutput(); err != nil {
			os.RemoveAll(bundleDir)
			d.logger.Dim("Command output:")
			d.logger.Dim(string(output))
			return "", fmt.Errorf("failed to pull bundle image: %w", err)
		}
	} else {
		d.logger.Dim("Bundle image already available locally, skipping pull")
	}

	containerID := fmt.Sprintf("stackrox-bundle-extract-%d", time.Now().Unix())

	createCmd := exec.CommandContext(ctx, containerTool, "create", "--name", containerID, bundleImage)
	if err := createCmd.Run(); err != nil {
		os.RemoveAll(bundleDir)
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	defer func() {
		rmCmd := exec.Command(containerTool, "rm", containerID)
		rmCmd.Run()
	}()

	cpCmd := exec.CommandContext(ctx, containerTool, "cp", fmt.Sprintf("%s:/manifests/.", containerID), bundleDir)
	if err := cpCmd.Run(); err != nil {
		os.RemoveAll(bundleDir)
		return "", fmt.Errorf("failed to copy bundle contents: %w", err)
	}

	d.logger.Successf("✓ Bundle extracted to: %s", bundleDir)
	return bundleDir, nil
}

// identifyCRDFiles identifies CRD files in the bundle directory
func (d *Deployer) identifyCRDFiles(bundleDir string) ([]string, error) {
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

		name := strings.ToLower(info.Name())
		if strings.Contains(name, "customresourcedefinition") ||
			strings.Contains(name, "platform.stackrox.io") ||
			strings.Contains(name, "config.stackrox.io") {
			if strings.Contains(name, "clusterserviceversion") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			if strings.Contains(string(content), "kind: CustomResourceDefinition") {
				crdFiles = append(crdFiles, path)
			}
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
		result, err := d.runKubectl(ctx, KubectlOptions{
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
	requiredCRDs := []string{
		"centrals.platform.stackrox.io",
		"securedclusters.platform.stackrox.io",
		"securitypolicies.config.stackrox.io",
	}

	var missing []string
	for _, crd := range requiredCRDs {
		_, err := d.runKubectl(ctx, KubectlOptions{
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

		crdFiles, err := d.identifyCRDFiles(bundleDir)
		if err != nil {
			return err
		}

		return d.applyCRDsToCluster(ctx, crdFiles)
	}

	return nil
}

func (d *Deployer) getOperatorBundleImage() string {
	return fmt.Sprintf("quay.io/rhacs-eng/stackrox-operator-bundle:v%s", d.operatorTag)
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
`, operatorNamespace, operatorNamespace)

	_, err := d.runKubectl(ctx, KubectlOptions{
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

	_, err = d.runKubectl(ctx, KubectlOptions{
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

	_, err = d.runKubectl(ctx, KubectlOptions{
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

	_, err = d.runKubectl(ctx, KubectlOptions{
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

	_, err = d.runKubectl(ctx, KubectlOptions{
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
		d.runKubectl(ctx, KubectlOptions{
			Args:         []string{"apply", "-n", namespace, "-f", serviceFile},
			IgnoreErrors: true,
		})
	}

	clusterRoleFile := filepath.Join(bundleDir, "rhacs-operator-metrics-reader_rbac.authorization.k8s.io_v1_clusterrole.yaml")
	if _, err := os.Stat(clusterRoleFile); err == nil {
		d.runKubectl(ctx, KubectlOptions{
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
		result, err := d.runKubectl(ctx, KubectlOptions{
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
