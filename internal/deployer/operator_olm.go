package deployer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stackrox/roxie/internal/k8s"
	"gopkg.in/yaml.v3"
)

const (
	catalogSourceName  = "stackrox-operator-index"
	subscriptionName   = "stackrox-operator-subscription"
	operatorGroupName  = "all-namespaces-operator-group"
	operatorChannel    = "latest"
	operatorIndexImage = "quay.io/rhacs-eng/stackrox-operator-index"
)

// OperatorDeploymentMode represents how the operator is deployed
type OperatorDeploymentMode bool

const (
	OperatorModeNonOLM OperatorDeploymentMode = false
	OperatorModeOLM    OperatorDeploymentMode = true
)

// deployOperatorViaOLM deploys the RHACS operator using OLM.
func (d *Deployer) deployOperatorViaOLM(ctx context.Context) error {
	d.logger.Info("🚀 Deploying operator via OLM...")
	d.logger.Infof("Operator tag: %s", d.operatorTag)

	if err := d.checkOLMInstalled(ctx); err != nil {
		return err
	}

	indexImage := d.getOperatorIndexImage()
	d.logger.Infof("Index image: %s", indexImage)

	if err := d.prepareNamespace(ctx, operatorNamespace, false); err != nil {
		return err
	}

	if err := d.createCatalogSource(ctx, indexImage); err != nil {
		return fmt.Errorf("failed to create CatalogSource: %w", err)
	}

	if err := d.createOperatorGroup(ctx); err != nil {
		return fmt.Errorf("failed to create OperatorGroup: %w", err)
	}

	if err := d.createSubscription(ctx); err != nil {
		return fmt.Errorf("failed to create Subscription: %w", err)
	}

	if err := d.waitForAndApproveInstallPlan(ctx); err != nil {
		return fmt.Errorf("failed to approve InstallPlan: %w", err)
	}

	if err := d.waitForCSVSuccess(ctx); err != nil {
		return fmt.Errorf("failed waiting for CSV: %w", err)
	}

	if err := d.waitForOperatorReady(ctx, operatorNamespace, operatorDeploymentName, 300); err != nil {
		return fmt.Errorf("failed waiting for operator: %w", err)
	}

	d.logger.Success("🎉 Operator deployed successfully via OLM!")
	return nil
}

// checkOLMInstalled checks if OLM is installed in the cluster by verifying
// the API server is ready to serve the required OLM resource types.
func (d *Deployer) checkOLMInstalled(ctx context.Context) error {
	requiredResources := []string{
		"catalogsources.operators.coreos.com",
		"subscriptions.operators.coreos.com",
		"installplans.operators.coreos.com",
		"clusterserviceversions.operators.coreos.com",
	}

	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"api-resources", "--api-group=operators.coreos.com", "-o", "name"},
	})
	if err != nil {
		if result.Stderr != "" {
			d.logger.Error("kubectl stderr:")
			for stderrLine := range strings.SplitSeq(result.Stderr, "\n") {
				d.logger.Errorf("stderr: %s", stderrLine)
			}
		}
		return fmt.Errorf("failed to query api-group operators.coreos.com: %w", err)
	}

	available := make(map[string]bool)
	for line := range strings.SplitSeq(strings.TrimSpace(result.Stdout), "\n") {
		name := strings.TrimSpace(line)
		available[name] = true
	}

	var missingResources []string
	for _, resource := range requiredResources {
		if !available[resource] {
			missingResources = append(missingResources, resource)
		}
	}
	if len(missingResources) > 0 {
		for _, resource := range missingResources {
			d.logger.Errorf("OLM resource not served by the API server: %s", resource)
		}
		return fmt.Errorf("OLM is not properly installed, %d required resource(s) missing", len(missingResources))
	}

	d.logger.Success("✓ OLM detected in cluster")
	return nil
}

// getOperatorIndexImage returns the operator index image reference.
func (d *Deployer) getOperatorIndexImage() string {
	return fmt.Sprintf(operatorIndexImage+":v%s", d.operatorTag)
}

// createCatalogSource creates the CatalogSource for the operator.
func (d *Deployer) createCatalogSource(ctx context.Context, indexImage string) error {
	d.logger.Info("Creating CatalogSource...")

	catalogSource := map[string]interface{}{
		"apiVersion": "operators.coreos.com/v1alpha1",
		"kind":       "CatalogSource",
		"metadata": map[string]interface{}{
			"name":      catalogSourceName,
			"namespace": operatorNamespace,
		},
		"spec": map[string]interface{}{
			"sourceType":  "grpc",
			"image":       indexImage,
			"displayName": "StackRox Operator Index",
			"grpcPodConfig": map[string]interface{}{
				"securityContextConfig": "restricted",
			},
		},
	}

	yamlData, err := yaml.Marshal(catalogSource)
	if err != nil {
		return fmt.Errorf("failed to marshal CatalogSource: %w", err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		// Apply with --validate=ignore because securityContextConfig may not
		// be in the CatalogSource CRD schema.
		Args:  []string{"apply", "--validate=ignore", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create CatalogSource: %w", err)
	}

	d.logger.Success("✓ CatalogSource created")
	return nil
}

// createOperatorGroup creates the OperatorGroup.
func (d *Deployer) createOperatorGroup(ctx context.Context) error {
	d.logger.Info("Creating OperatorGroup...")

	operatorGroup := map[string]interface{}{
		"apiVersion": "operators.coreos.com/v1alpha2",
		"kind":       "OperatorGroup",
		"metadata": map[string]interface{}{
			"name":      operatorGroupName,
			"namespace": operatorNamespace,
		},
	}

	yamlData, err := yaml.Marshal(operatorGroup)
	if err != nil {
		return fmt.Errorf("failed to marshal OperatorGroup: %w", err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create OperatorGroup: %w", err)
	}

	d.logger.Success("✓ OperatorGroup created")
	return nil
}

// createSubscription creates the Subscription for the operator.
func (d *Deployer) createSubscription(ctx context.Context) error {
	d.logger.Info("Creating Subscription...")

	startingCSV := fmt.Sprintf("rhacs-operator.v%s", d.operatorTag)

	subscription := map[string]interface{}{
		"apiVersion": "operators.coreos.com/v1alpha1",
		"kind":       "Subscription",
		"metadata": map[string]interface{}{
			"name":      subscriptionName,
			"namespace": operatorNamespace,
		},
		"spec": map[string]interface{}{
			"channel":             operatorChannel,
			"name":                "rhacs-operator",
			"source":              catalogSourceName,
			"sourceNamespace":     operatorNamespace,
			"installPlanApproval": "Manual",
			"startingCSV":         startingCSV,
		},
	}

	yamlData, err := yaml.Marshal(subscription)
	if err != nil {
		return fmt.Errorf("failed to marshal Subscription: %w", err)
	}

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create Subscription: %w", err)
	}

	d.logger.Success("✓ Subscription created")
	return nil
}

// waitForAndApproveInstallPlan waits for the InstallPlan to be created and approves it.
func (d *Deployer) waitForAndApproveInstallPlan(ctx context.Context) error {
	d.logger.Info("⏳ Waiting for InstallPlan to be created...")

	// Wait for subscription to have InstallPlanPending condition.
	start := time.Now()
	timeout := 5 * time.Minute

	for time.Since(start) < timeout {
		result, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "subscription", subscriptionName, "-n", operatorNamespace, "-o", "jsonpath={.status.conditions[?(@.type=='InstallPlanPending')].status}"},
		})
		if err == nil && strings.TrimSpace(result.Stdout) == "True" {
			break
		}

		time.Sleep(5 * time.Second)
	}

	if time.Since(start) >= timeout {
		// TODO(ROX-34499): some more info on what was wrong would be useful: a dump of the
		// subscription or at least its name so that the user can investigate
		return errors.New("timeout waiting for InstallPlan to be created")
	}

	// Sanity check:Verify currentCSV matches expected version.
	expectedCSV := fmt.Sprintf("rhacs-operator.v%s", d.operatorTag)
	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "subscription", subscriptionName, "-n", operatorNamespace, "-o", "jsonpath={.status.currentCSV}"},
	})
	if err != nil {
		return fmt.Errorf("failed to get current CSV from subscription: %w", err)
	}

	currentCSV := strings.TrimSpace(result.Stdout)
	if currentCSV != expectedCSV {
		return fmt.Errorf("subscription progressing to unexpected CSV '%s', expected '%s'", currentCSV, expectedCSV)
	}

	// Get InstallPlan name.
	result, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "subscription", subscriptionName, "-n", operatorNamespace, "-o", "jsonpath={.status.installPlanRef.name}"},
	})
	if err != nil {
		return fmt.Errorf("failed to get InstallPlan name: %w", err)
	}

	installPlanName := strings.TrimSpace(result.Stdout)
	if installPlanName == "" {
		return errors.New("InstallPlan name is empty")
	}

	d.logger.Infof("Approving InstallPlan: %s", installPlanName)

	// Approve the InstallPlan.
	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"patch", "installplan", installPlanName, "-n", operatorNamespace, "--type", "merge", "-p", `{"spec":{"approved":true}}`},
	})
	if err != nil {
		return fmt.Errorf("failed to approve InstallPlan: %w", err)
	}

	d.logger.Success("✓ InstallPlan approved")
	return nil
}

// waitForCSVSuccess waits for the CSV to reach Succeeded phase.
func (d *Deployer) waitForCSVSuccess(ctx context.Context) error {
	csvName := fmt.Sprintf("rhacs-operator.v%s", d.operatorTag)
	d.logger.Infof("⏳ Waiting for CSV %s to succeed...", csvName)

	start := time.Now()
	timeout := 10 * time.Minute

	for time.Since(start) < timeout {
		result, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "csv", csvName, "-n", operatorNamespace, "-o", "jsonpath={.status.phase}"},
		})
		if err == nil {
			phase := strings.TrimSpace(result.Stdout)
			if phase == "Succeeded" {
				d.logger.Success("✓ CSV succeeded")
				return nil
			}
			if phase == "Failed" {
				return fmt.Errorf("CSV entered Failed phase")
			}
		}

		time.Sleep(5 * time.Second)
	}

	// TODO(ROX-34499): same as above
	return fmt.Errorf("timeout waiting for CSV to succeed")
}

// detectOperatorDeploymentMode detects how the operator is currently deployed.
// Returns (operatorExists bool, isOLM OperatorDeploymentMode)
func (d *Deployer) detectOperatorDeploymentMode(ctx context.Context) (bool, OperatorDeploymentMode, error) {
	const olmOwnerLabel = "olm.owner"

	// First, check if a Subscription exists (OLM-specific resource)
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "subscription", subscriptionName, "-n", operatorNamespace},
	})
	if err == nil {
		return true, OperatorModeOLM, nil
	}

	// If no subscription, check if operator deployment exists/if it has the expected OLM label.
	labelValue, err := k8s.RetrieveClusterResourceLabel(ctx, d.logger, operatorNamespace, "deployment", operatorDeploymentName, olmOwnerLabel)
	if k8s.IsResourceNotFound(err) {
		// No operator deployment found.
		return false, OperatorModeNonOLM, nil
	}
	if err != nil {
		return false, OperatorModeNonOLM, err
	}

	if labelValue == "" {
		// Deployment exists without OLM labels -> non-OLM deployment.
		return true, OperatorModeNonOLM, nil
	}

	// Label set -> OLM deployment.
	return true, OperatorModeOLM, nil
}

// teardownOperatorOLM removes the operator when installed via OLM.
func (d *Deployer) teardownOperatorOLM(ctx context.Context) error {
	d.logger.Info("🧹 Tearing down operator deployed via OLM...")

	// Delete Subscription (this typically cascades CSV and operands depending on OLM behavior).
	d.runKubectl(ctx, k8s.KubectlOptions{
		Args:         []string{"delete", "subscription", subscriptionName, "-n", operatorNamespace, "--ignore-not-found=true"},
		IgnoreErrors: true,
	})

	// Find the CSV name (may match operatorTag, but query to be safe).
	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "csv", "-n", operatorNamespace, "-o", "jsonpath={.items[*].metadata.name}"},
	})
	if err == nil {
		// Best-effort delete all matching CSVs for rhacs-operator.
		for _, name := range strings.Fields(strings.TrimSpace(result.Stdout)) {
			if strings.HasPrefix(name, "rhacs-operator.v") {
				d.runKubectl(ctx, k8s.KubectlOptions{
					Args:         []string{"delete", "csv", name, "-n", operatorNamespace, "--ignore-not-found=true"},
					IgnoreErrors: true,
				})
			}
		}
	}

	// Delete CatalogSource and OperatorGroup.
	d.runKubectl(ctx, k8s.KubectlOptions{
		Args:         []string{"delete", "catalogsource", catalogSourceName, "-n", operatorNamespace, "--ignore-not-found=true"},
		IgnoreErrors: true,
	})
	d.runKubectl(ctx, k8s.KubectlOptions{
		Args:         []string{"delete", "operatorgroup", operatorGroupName, "-n", operatorNamespace, "--ignore-not-found=true"},
		IgnoreErrors: true,
	})

	// Delete operator deployment namespace (contains deployment, SA, etc.).
	d.runKubectl(ctx, k8s.KubectlOptions{
		Args:         []string{"delete", "namespace", operatorNamespace, "--ignore-not-found=true", "--wait=false"},
		IgnoreErrors: true,
	})

	if err := d.waitForNamespaceDeletion(operatorNamespace); err != nil {
		d.logger.Warningf("Namespace %s deletion incomplete: %v", operatorNamespace, err)
	}

	d.logger.Success("✓ OLM operator resources removed")
	return nil
}
