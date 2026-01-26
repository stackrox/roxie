package deployer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"

	"github.com/stackrox/roxie/internal/cluster"
	"github.com/stackrox/roxie/internal/clusterdefaults"
	"github.com/stackrox/roxie/internal/dockerauth"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/imagecache"
	"github.com/stackrox/roxie/internal/localimages"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/portforward"
)

const (
	// totalRequiredImages is the number of images needed for a complete deployment
	// (7 main images + 2 operator images: stackrox-operator + stackrox-operator-bundle).
	// Main images: main, scanner, scanner-db, scanner-v4, scanner-v4-db, central-db, collector
	// Note: scanner-v4 is a single image that runs in different modes (indexer/matcher)
	//       based on runtime configuration.
	// Note: operator-index is not included as roxie doesn't use it in default (non-OLM) mode.
	totalRequiredImages = 9

	// skipLocalImagesEnvVar is the environment variable name to disable local image detection.
	skipLocalImagesEnvVar = "ROXIE_SKIP_LOCAL_IMAGES"
)

var (
	sharedNamespace  = "stackrox"
	centralNamespace = "acs-central"
	sensorNamespace  = "acs-sensor"
	defaultExposure  = "loadbalancer"

	pauseReconcileAnnotationKey = "stackrox.io/pause-reconcile"

	allInstallableCentralResourceKinds = []string{
		"applications",
		"clusterroles",
		"configmaps",
		"deployments",
		"destinationrules",
		"endpoints",
		"endpointslices",
		"horizontalpodautoscalers",
		"networkpolicys",
		"leases",
		"persistentvolumes",
		"persistentvolumeclaims",
		"pods",
		"podsecuritypolicys",
		"prometheusrules",
		"roles",
		"rolebindings",
		"replicasets",
		"routes",
		"secrets",
		"services",
		"serviceaccounts",
		"servicemonitors",
		"storageclasses",
	}

	allInstallableSecuredClusterResourceKinds = []string{
		"clusterroles",
		"clusterrolebindings",
		"configmaps",
		"consoleplugins",
		"controllerrevisions",
		"daemonsets",
		"deployments",
		"endpoints",
		"endpointslices",
		"destinationrules",
		"horizontalpodautoscalers",
		"networkpolicys",
		"leases",
		"persistentvolumes",
		"persistentvolumeclaims",
		"pods",
		"podsecuritypolicys",
		"prometheusrules",
		"replicasets",
		"roles",
		"rolebindings",
		"secrets",
		"services",
		"serviceaccounts",
		"servicemonitors",
		"storageclasses",
		"validatingwebhookconfigurations",
	}
)

// Deployer is the base deployer for ACS
type Deployer struct {
	logger                 *logger.Logger
	startTime              time.Time
	dockerAuth             *dockerauth.DockerAuth
	imageCache             *imagecache.ImageCache
	portForward            *portforward.Manager
	clusterDefaults        *clusterdefaults.Manager
	kubectl                string
	roxctlVersion          string
	centralNamespace       string
	sensorNamespace        string
	mainImageTag           string
	operatorTag            string
	centralEndpoint        string
	centralPassword        string
	roxCACertFile          string
	kubeContext            string
	portForwardEnabled     bool
	pauseReconciliation    bool
	exposure               string
	overrideFile           string
	overrideSetExpressions []string
	envrcFile              string
	useHelm                bool
	useOLM                 bool
	useKonflux             bool
	shouldDeployOperator   bool
	verbose                bool
	earlyReadiness         bool
	dockerCreds            *dockerauth.Credentials
	clusterResourceKinds   map[string]struct{}
	// New fields for local image support
	localImages      map[string]string // map of image names to local references
	usingLocalImages bool              // true if any local images were found and loaded
}

type ResourceKindWithName struct {
	Kind string
	Name string
}

func (d *Deployer) filterResourceKinds(resourceKinds []string) []string {
	filteredResourceKinds := make([]string, 0, len(resourceKinds))
	for _, resourceKind := range resourceKinds {
		if _, ok := d.clusterResourceKinds[resourceKind]; ok {
			filteredResourceKinds = append(filteredResourceKinds, resourceKind)
		}
	}
	return filteredResourceKinds
}

func (d *Deployer) deleteResource(ctx context.Context, namespace, resourceType, resourceName string, args ...string) error {
	finalArgs := []string{
		"-n", namespace,
		"delete",
		resourceType,
		resourceName,
		"--ignore-not-found",
		"--force",
		"--grace-period=0",
	}
	finalArgs = append(finalArgs, args...)
	_, err := d.runKubectl(ctx, KubectlOptions{Args: finalArgs})
	return err
}

func (d *Deployer) deleteResources(ctx context.Context, namespace string, resourceTypes []string, args ...string) error {
	resourceTypesString := strings.Join(resourceTypes, ",")
	finalArgs := []string{
		"-n", namespace,
		"delete",
		resourceTypesString,
		"--ignore-not-found",
		"--force",
		"--grace-period=0",
	}
	finalArgs = append(finalArgs, args...)
	_, err := d.runKubectl(ctx, KubectlOptions{Args: finalArgs})
	return err
}

func (d *Deployer) deleteFinalizers(ctx context.Context, namespace, resourceType, resourceName string) error {
	_, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{
			"-n", namespace, "patch", resourceType, resourceName,
			"-p", "{\"metadata\":{\"finalizers\":null}}",
			"--type=merge",
		},
	})
	return err
}

// Expects that reconciliation for the RHACS operator is paused.
func (d *Deployer) deleteCentralResources(ctx context.Context, wait bool) error {
	d.logger.Info("Deleting Central resources")
	var crExists bool

	if d.doesResourceExist(ctx, "central", "stackrox-central-services", d.centralNamespace) {
		crExists = true

		// Trigger async deletion of the Central CR.
		err := d.deleteResource(ctx, d.centralNamespace, "central", "stackrox-central-services", "--wait=false")
		if err != nil {
			return fmt.Errorf("failed to asynchronously delete Central CR: %w", err)
		}

		err = d.deleteFinalizers(ctx, d.centralNamespace, "central", "stackrox-central-services")
		if err != nil {
			return fmt.Errorf("failed to delete finalizers on Central CR: %w", err)
		}
	}

	// Pause reconciliation for other controllers, not just our RHACS operator.
	// This is needed to ensure that there is no race causing the Cluster Network Operator
	// to re-create the injected-ca-bundle ConfigMap during resource deletion.
	err := d.preventOtherControllersFromReconciling(ctx)
	if err != nil {
		return fmt.Errorf("failed to prevent other controllers from reconciling: %w", err)
	}

	// Delete other resources by brute force.
	resourceKinds := d.filterResourceKinds(allInstallableCentralResourceKinds)
	err = d.deleteResources(ctx, d.centralNamespace, resourceKinds, "-l=app.kubernetes.io/part-of=stackrox-central-services")
	if err != nil {
		return err
	}

	for _, resource := range []ResourceKindWithName{
		{Name: "central-db", Kind: "pvc"},
		{Name: "central-db-backup", Kind: "pvc"},
		{Name: "admin-password", Kind: "secret"},
	} {
		err := d.deleteResource(ctx, d.centralNamespace, resource.Kind, resource.Name)
		if err != nil {
			return fmt.Errorf("failed to delete %s/%s: %w", resource.Kind, resource.Name, err)
		}
	}

	if crExists {
		// Now delete the Central CR synchronously.
		err := d.deleteResource(ctx, d.centralNamespace, "central", "stackrox-central-services")
		if err != nil {
			return fmt.Errorf("failed to delete Central CR: %w", err)
		}
	}

	return nil
}

func (d *Deployer) preventOtherControllersFromReconciling(ctx context.Context) error {
	return d.preventCABundleInjection(ctx)
}

func (d *Deployer) preventCABundleInjection(ctx context.Context) error {
	configMapName := "injected-cabundle-stackrox-central-services"

	if !d.doesResourceExist(ctx, "configmap", configMapName, d.centralNamespace) {
		return nil
	}

	d.logger.Info("Removing CNO label from injected-cabundle ConfigMap to prevent CNO from injecting the CA bundle during cleanup")
	_, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{
			"label", "configmap", configMapName, "-n", d.centralNamespace,
			"config.openshift.io/inject-trusted-cabundle-",
		},
		IgnoreErrors: true,
	})

	if err != nil {
		d.logger.Warningf("Failed to remove CNO label from %s: %v", configMapName, err)
	}

	return nil
}

func (d *Deployer) deleteSecuredClusterResources(ctx context.Context, wait bool) error {
	d.logger.Info("Deleting SecuredCluster resources")
	var crExists bool

	if d.doesResourceExist(ctx, "securedcluster", "stackrox-secured-cluster-services", d.sensorNamespace) {
		crExists = true

		// Trigger async deletion of the SecuredCluster CR.
		err := d.deleteResource(ctx, d.sensorNamespace, "securedcluster", "stackrox-secured-cluster-services", "--wait=false")
		if err != nil {
			return err
		}

		err = d.deleteFinalizers(ctx, d.sensorNamespace, "securedcluster", "stackrox-secured-cluster-services")
		if err != nil {
			return fmt.Errorf("failed to delete finalizers on SecuredCluster CR: %w", err)
		}
	}

	// In the meantime, delete other resources by brute force.
	resourceKinds := d.filterResourceKinds(allInstallableSecuredClusterResourceKinds)
	err := d.deleteResources(ctx, d.sensorNamespace, resourceKinds, "-l=app.kubernetes.io/part-of=stackrox-secured-cluster-services")
	if err != nil {
		return err
	}

	for _, resource := range []ResourceKindWithName{
		{Name: "cluster-registration-secret", Kind: "secret"},
		{Name: "scanner-db-password", Kind: "secret"},
	} {
		err := d.deleteResource(ctx, d.sensorNamespace, resource.Kind, resource.Name)
		if err != nil {
			return fmt.Errorf("failed to delete %s/%s: %w", resource.Kind, resource.Name, err)
		}
	}

	if crExists {
		// Now delete the SecuredCluster CR synchronously.
		err := d.deleteResource(ctx, d.sensorNamespace, "securedcluster", "stackrox-secured-cluster-services")
		if err != nil {
			return fmt.Errorf("failed to delete SecuredCluster CR: %w", err)
		}
	}

	return nil
}

func New(log *logger.Logger, overrideFile string, overrideSetExpressions []string) (*Deployer, error) {
	// Check required tools first
	if err := checkRequiredTools(); err != nil {
		return nil, err
	}

	roxctlVersion, err := getRoxctlVersion()
	if err != nil {
		return nil, err
	}

	kubectl := getKubectl()

	d := &Deployer{
		logger:                 log,
		startTime:              time.Now(),
		kubectl:                kubectl,
		roxctlVersion:          roxctlVersion,
		centralNamespace:       centralNamespace,
		sensorNamespace:        sensorNamespace,
		exposure:               defaultExposure,
		overrideFile:           overrideFile,
		overrideSetExpressions: overrideSetExpressions,
		shouldDeployOperator:   true,
	}

	d.dockerAuth = dockerauth.New(log)
	d.imageCache = imagecache.New(log, "", 20)
	d.portForward = portforward.New(kubectl, log)
	d.clusterDefaults = clusterdefaults.NewManager(log)

	if password := os.Getenv("ROX_ADMIN_PASSWORD"); password != "" {
		d.centralPassword = password
	} else {
		d.centralPassword = generatePassword()
	}

	if endpoint := os.Getenv("API_ENDPOINT"); endpoint != "" {
		d.centralEndpoint = endpoint
	}

	if caCert := os.Getenv("ROX_CA_CERT_FILE"); caCert != "" {
		d.roxCACertFile = caCert
	}

	ctx, err := getCurrentContext(kubectl)
	if err != nil {
		return nil, err
	}
	d.kubeContext = ctx

	clusterResourceKinds, err := d.getClusterResourceKinds()
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster resource kinds: %w", err)
	}
	d.clusterResourceKinds = clusterResourceKinds

	log.Success("🚀 ACS Deployer initialized")
	log.Infof("roxctl version: %s", d.roxctlVersion)

	return d, nil
}

func (d *Deployer) getClusterResourceKinds() (map[string]struct{}, error) {
	result, err := d.runKubectl(context.Background(), KubectlOptions{
		Args: []string{"api-resources", "-o", "name"},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster resource kinds: %w", err)
	}
	kinds := make(map[string]struct{})
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, line := range lines {
		fields := strings.SplitN(line, ".", 2)
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		kind := fields[0]
		kinds[kind] = struct{}{}
	}

	return kinds, nil
}

func formatComponentName(component string) string {
	switch component {
	case "both", "all":
		return "Central and Secured Cluster"
	case "secured-cluster", "sensor":
		return "Secured Cluster"
	case "central":
		return "Central"
	case "operator":
		return "Operator"
	default:
		return component
	}
}

func (d *Deployer) Deploy(ctx context.Context, component, resources, exposure string) error {
	adjustedResources, adjustedExposure, adjustedPortForward := d.clusterDefaults.ApplyConvenienceDefaults(
		d.kubeContext,
		resources,
		exposure,
		d.portForwardEnabled,
	)

	resources = adjustedResources
	exposure = adjustedExposure
	d.portForwardEnabled = adjustedPortForward
	d.exposure = exposure

	// Detect and load local images for kind clusters
	if err := d.detectAndLoadLocalImages(ctx); err != nil {
		return fmt.Errorf("failed to detect and load local images: %w", err)
	}

	// Prepare and verify credentials early to fail fast
	// Skip if all images are local
	if !d.shouldSkipCredentialVerification() {
		if err := d.prepareCredentials(); err != nil {
			return fmt.Errorf("failed to prepare credentials: %w", err)
		}
	} else {
		d.logger.Info("All images loaded locally, skipping credential verification")
	}

	d.logger.Infof("Initiating deployment of %s", formatComponentName(component))

	switch component {
	case "operator":
		return d.deployOperatorOnly(ctx)
	case "central":
		return d.deployCentral(ctx, resources, exposure)
	case "secured-cluster", "sensor":
		return d.deploySecuredCluster(ctx, resources)
	case "both", "all":
		if err := d.deployCentral(ctx, resources, exposure); err != nil {
			return fmt.Errorf("failed to deploy central: %w", err)
		}
		return d.deploySecuredCluster(ctx, resources)
	default:
		return fmt.Errorf("unknown component: %s", component)
	}
}

// prepareCredentials prepares and verifies Docker credentials early to fail fast.
// The verified credentials are stored for later use.
func (d *Deployer) prepareCredentials() error {
	d.logger.Dimf("Preparing and verifying Docker credentials...")

	// This will retrieve and verify credentials, returning error if invalid
	creds, err := d.dockerAuth.GetAndVerifyCredentials()
	if err != nil {
		return err
	}

	// Store the verified credentials
	d.dockerCreds = creds

	d.logger.Dimf("Docker credentials verified successfully")
	return nil
}

// detectAndLoadLocalImages attempts to detect and load locally-available container
// images into the kind cluster. It performs the following steps:
//  1. Checks if ROXIE_SKIP_LOCAL_IMAGES environment variable is set
//  2. Verifies the target cluster is a kind cluster
//  3. Queries podman for locally available images
//  4. Loads discovered images into the kind cluster
//
// This method gracefully degrades - if podman is unavailable or no images are found,
// it returns nil and allows deployment to proceed with remote image pulls.
// Returns an error only if image loading into kind fails after images are detected.
func (d *Deployer) detectAndLoadLocalImages(ctx context.Context) error {
	// Check if ROXIE_SKIP_LOCAL_IMAGES is set
	if os.Getenv(skipLocalImagesEnvVar) == "true" {
		d.logger.Dim("ROXIE_SKIP_LOCAL_IMAGES is set, skipping local image detection")
		return nil
	}

	// Check if this is a kind cluster
	if !cluster.IsKindCluster() {
		d.logger.Dim("Not a kind cluster, skipping local image detection")
		return nil
	}

	kindClusterName := cluster.GetKindClusterName()
	d.logger.Infof("Detected kind cluster: %s", kindClusterName)

	// Check for local images
	d.logger.Dim("Checking for local images in podman...")
	localImages, err := localimages.CheckImages(d.mainImageTag, d.operatorTag)
	if err != nil {
		// If podman is not available, gracefully fall back
		d.logger.Dimf("Could not check for local images: %v", err)
		d.logger.Dim("Falling back to quay.io")
		return nil
	}

	if len(localImages) == 0 {
		d.logger.Dim("No local images found, will pull from quay.io")
		return nil
	}

	// Calculate total images needed (7 main + 2 operator = 9)
	totalExpected := totalRequiredImages
	d.logger.Infof("Found %d/%d images locally in podman", len(localImages), totalExpected)

	// Load images into kind using quay.io paths
	if err := localimages.LoadImagesToKind(ctx, localImages, d.mainImageTag, d.operatorTag, kindClusterName, d.logger); err != nil {
		return fmt.Errorf("failed to load %d images into kind cluster %s: %w", len(localImages), kindClusterName, err)
	}

	// Store the local images for later use
	d.localImages = localImages
	d.usingLocalImages = len(localImages) > 0

	return nil
}

// shouldSkipCredentialVerification returns true if credential verification should be skipped.
// Verification is skipped only when all required images (9 total) are available locally.
// For partial local image scenarios, verification is still required for remote pulls.
func (d *Deployer) shouldSkipCredentialVerification() bool {
	// If not using any local images, don't skip
	if !d.usingLocalImages {
		return false
	}

	// If using some local images but not all, don't skip (need creds for remote pulls)
	// Total expected: 7 main + 2 operator = 9
	totalExpected := totalRequiredImages
	if len(d.localImages) < totalExpected {
		d.logger.Dimf("Using %d/%d local images, remaining images will be pulled from quay.io",
			len(d.localImages), totalExpected)
		return false
	}

	// All images are local
	return true
}

// shouldSkipImagePullSecrets returns true if image pull secrets should be skipped.
// Same logic as credential verification - skip only if all images are local.
func (d *Deployer) shouldSkipImagePullSecrets() bool {
	return d.shouldSkipCredentialVerification()
}

func (d *Deployer) deployCentral(ctx context.Context, resources, exposure string) error {
	d.logger.Infof("Deploying Central to namespace %s", d.centralNamespace)
	if d.namespaceExists(d.centralNamespace) {
		d.logger.Info("Existing Central deployment found, tearing down...")
		if err := d.teardownCentral(ctx); err != nil {
			d.logger.Warningf("Error during teardown: %v", err)
		}
	}

	portForwardWanted := d.portForwardEnabled

	var err error
	if d.useHelm {
		err = d.deployCentralHelm(ctx, resources, exposure)
	} else {
		err = d.deployCentralOperator(ctx, resources, exposure)
	}

	if err != nil {
		return err
	}

	// envrc may be used from different processes, so use actual endpoint not port-forward
	if d.envrcFile != "" {
		if err := d.writeEnvrcFile(ctx, exposure, portForwardWanted); err != nil {
			d.logger.Warningf("Failed to write envrc file: %v", err)
		}
	}

	return nil
}

func (d *Deployer) deploySecuredCluster(ctx context.Context, resources string) error {
	d.logger.Infof("Deploying SecuredCluster to namespace %s", d.sensorNamespace)
	if d.namespaceExists(d.sensorNamespace) {
		d.logger.Info("Existing SecuredCluster deployment found, tearing down...")
		if err := d.teardownSecuredCluster(ctx); err != nil {
			d.logger.Warningf("Error during teardown: %v", err)
		}
	}

	if d.useHelm {
		return d.deploySecuredClusterHelm(ctx, resources)
	}
	return d.deploySecuredClusterOperator(ctx, resources)
}

func (d *Deployer) Teardown(ctx context.Context, component string) error {
	d.logger.Infof("Starting teardown of %s", component)

	switch component {
	case "central":
		return d.teardownCentral(ctx)
	case "secured-cluster", "sensor":
		return d.teardownSecuredCluster(ctx)
	case "both", "all":
		if err := d.teardownSecuredCluster(ctx); err != nil {
			d.logger.Warningf("Error tearing down secured cluster: %v", err)
		}
		return d.teardownCentral(ctx)
	default:
		return fmt.Errorf("unknown component: %s", component)
	}
}

func (d *Deployer) teardownCentral(ctx context.Context) error {
	d.logger.Infof("🗑️  Tearing down %s", d.centralNamespace)

	if !d.namespaceExists(d.centralNamespace) {
		d.logger.Infof("Namespace %s doesn't exist, skipping", d.centralNamespace)
		return nil
	}

	d.portForward.Stop()

	// Add pause-reconcile annotation to not have the operator interfere during resource deletion.
	if d.doesResourceExist(ctx, "central", "stackrox-central-services", d.centralNamespace) {
		if err := d.addPauseReconcileAnnotation(ctx, "central", "stackrox-central-services", d.centralNamespace); err != nil {
			d.logger.Warningf("Error adding pause-reconcile annotation: %v", err)
		}
	}

	d.logger.Info("⏳ Waiting for Central resources to be fully deleted...")
	err := d.deleteCentralResources(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to delete Central resources: %w", err)
	}

	d.logger.Successf("✓ Central resources in namespace %s have been deleted", d.centralNamespace)
	return nil
}

func (d *Deployer) teardownSecuredCluster(ctx context.Context) error {
	d.logger.Infof("🗑️  Tearing down %s", d.sensorNamespace)

	if !d.namespaceExists(d.sensorNamespace) {
		d.logger.Infof("Namespace %s doesn't exist, skipping", d.sensorNamespace)
		return nil
	}

	if d.doesResourceExist(ctx, "securedcluster", "stackrox-secured-cluster-services", d.sensorNamespace) {
		// Add pause-reconcile annotation to not have the operator interfere during resource deletion.
		if err := d.addPauseReconcileAnnotation(ctx, "securedcluster", "stackrox-secured-cluster-services", d.sensorNamespace); err != nil {
			d.logger.Warningf("Error adding pause-reconcile annotation: %v", err)
		}
	}

	d.logger.Info("⏳ Waiting for SecuredCluster resources to be fully deleted...")
	err := d.deleteSecuredClusterResources(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to delete SecuredCluster resources: %w", err)
	}

	d.logger.Successf("✓ SecuredCluster resources in namespace %s have been deleted", d.sensorNamespace)
	return nil
}

func (d *Deployer) ensureNamespaceExists(namespace string) error {
	if d.namespaceExists(namespace) {
		return nil
	}

	d.logger.Infof("Creating namespace %s", namespace)

	_, err := d.runKubectl(context.Background(), KubectlOptions{
		Args: []string{"create", "namespace", namespace},
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}

func (d *Deployer) namespaceExists(namespace string) bool {
	_, err := d.runKubectl(context.Background(), KubectlOptions{
		Args: []string{"get", "namespace", namespace},
	})
	return err == nil
}

func (d *Deployer) waitForNamespaceDeletion(namespace string) error {
	timeout := 5 * time.Minute
	checkInterval := 2 * time.Second
	progressInterval := 10 * time.Second // Report progress every 10 seconds
	deadline := time.Now().Add(timeout)
	lastProgressReport := time.Now()

	for time.Now().Before(deadline) {
		if !d.namespaceExists(namespace) {
			d.logger.Infof("Namespace %s has been deleted", namespace)
			return nil
		}

		// Report progress periodically
		if time.Since(lastProgressReport) >= progressInterval {
			elapsed := time.Since(deadline.Add(-timeout))
			d.logger.Dim(fmt.Sprintf("  ⋯ Still waiting for namespace deletion... (%.0fs elapsed)", elapsed.Seconds()))
			lastProgressReport = time.Now()
		}

		time.Sleep(checkInterval)
	}

	return fmt.Errorf("timeout waiting for namespace %s to be deleted", namespace)
}

// checkRequiredTools verifies that required CLI tools are available
func checkRequiredTools() error {
	requiredTools := []string{"kubectl", "roxctl"}

	var missing []string
	for _, tool := range requiredTools {
		if _, err := exec.LookPath(tool); err != nil {
			missing = append(missing, tool)
		}
	}

	// Check for container tool (podman or docker)
	containerTool := ""
	if _, err := exec.LookPath("podman"); err == nil {
		containerTool = "podman"
	} else if _, err := exec.LookPath("docker"); err == nil {
		containerTool = "docker"
	}

	if containerTool == "" {
		missing = append(missing, "podman or docker")
	}

	if len(missing) > 0 {
		return fmt.Errorf("required tools not found in PATH: %s\nPlease install these tools and ensure they are available in your PATH", strings.Join(missing, ", "))
	}

	return nil
}

func getKubectl() string {
	if kubectl := os.Getenv("ORCH_CMD"); kubectl != "" {
		return kubectl
	}
	return "kubectl"
}

func getRoxctlVersion() (string, error) {
	cmd := exec.Command("roxctl", "version")
	output, err := cmd.Output()
	if err != nil {
		if _, ok := err.(*exec.Error); ok {
			return "", errors.New("roxctl not found in PATH; please install roxctl and ensure it's available in your PATH")
		}
		if _, ok := err.(*exec.ExitError); ok {
			return "", errors.New("roxctl not found in PATH; please install roxctl and ensure it's available in your PATH")
		}
		return "", fmt.Errorf("failed to get roxctl version: %w", err)
	}

	version := strings.TrimSpace(string(output))
	if version == "" {
		return "", errors.New("roxctl returned empty version")
	}

	return version, nil
}

func getCurrentContext(kubectl string) (string, error) {
	cmd := exec.Command(kubectl, "config", "current-context")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current context: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

func generatePassword() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	const passwordLength = 20

	password := make([]byte, passwordLength)
	randomBytes := make([]byte, passwordLength)

	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("admin-%d", time.Now().Unix())
	}

	for i := 0; i < passwordLength; i++ {
		password[i] = charset[int(randomBytes[i])%len(charset)]
	}

	return string(password)
}

func (d *Deployer) SetEnvrcFile(path string) {
	d.envrcFile = path
}

func (d *Deployer) SetPortForwardingEnabled(enabled bool) {
	d.portForwardEnabled = enabled
}

func (d *Deployer) SetUseHelm(useHelm bool) error {
	if useHelm {
		if _, err := exec.LookPath("helm"); err != nil {
			return errors.New("helm not found in PATH; please install helm and ensure it's available in your PATH when using --helm flag")
		}
	}
	d.useHelm = useHelm
	return nil
}

func (d *Deployer) SetUseOLM(useOLM bool) error {
	d.useOLM = useOLM
	return nil
}

func (d *Deployer) SetUseKonflux(useKonflux bool) error {
	d.useKonflux = useKonflux
	return nil
}

func (d *Deployer) SetVerbose(verbose bool) {
	d.verbose = verbose
}

func (d *Deployer) SetEarlyReadiness(enabled bool) {
	d.earlyReadiness = enabled
}

func (d *Deployer) SetPauseReconciliation(enabled bool) {
	d.pauseReconciliation = enabled
}

func (d *Deployer) SetSingleNamespace(enabled bool) {
	if enabled {
		d.centralNamespace = sharedNamespace
		d.sensorNamespace = sharedNamespace
	}
}

func (d *Deployer) SetMainImageTag(tag string) {
	d.mainImageTag = tag
	d.operatorTag = helpers.ConvertMainTagToOperatorTag(d.mainImageTag)
}

// maybeAddPauseReconcileAnnotation adds the stackrox.io/pause-reconcile annotation to a custom resource
func (d *Deployer) maybeAddPauseReconcileAnnotation(ctx context.Context, resourceType, resourceName, namespace string) error {
	if !d.pauseReconciliation {
		return nil
	}

	d.logger.Infof("Adding pause-reconcile annotation to %s/%s", resourceType, resourceName)

	err := d.addPauseReconcileAnnotation(ctx, resourceType, resourceName, namespace)
	if err != nil {
		return err
	}

	d.logger.Successf("✓ Added pause-reconcile annotation to %s/%s", resourceType, resourceName)
	return nil
}

func (d *Deployer) doesResourceExist(ctx context.Context, resourceType, resourceName, namespace string) bool {
	_, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{
			"get", resourceType, resourceName,
			"-n", namespace,
		},
	})
	return err == nil
}

func (d *Deployer) addPauseReconcileAnnotation(ctx context.Context, resourceType, resourceName, namespace string) error {
	_, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{
			"annotate", resourceType, resourceName,
			"-n", namespace,
			fmt.Sprintf("%s=%s", pauseReconcileAnnotationKey, "true"),
			"--overwrite",
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add pause-reconcile annotation: %w", err)
	}

	return nil
}

func (d *Deployer) SetDeployOperator(deployOperator bool) {
	d.shouldDeployOperator = deployOperator
}

func (d *Deployer) GetDeploymentInfo() (endpoint, password, caCertFile, kubeContext, exposure string) {
	return d.centralEndpoint, d.centralPassword, d.roxCACertFile, d.kubeContext, d.exposure
}

// WaitForCentral waits for Central to be ready and responding on its endpoint
// Returns true if Central is ready, false if timeout occurs
func (d *Deployer) WaitForCentral(timeout time.Duration) bool {
	if d.centralEndpoint == "" {
		d.logger.Dim("No Central endpoint configured, skipping readiness check")
		return false
	}

	d.logger.Infof("⏳ Waiting for Central to be ready at %s (timeout: %v)", d.centralEndpoint, timeout)

	deadline := time.Now().Add(timeout)
	checkInterval := 5 * time.Second
	progressInterval := 30 * time.Second
	lastProgressReport := time.Now()

	for time.Now().Before(deadline) {
		// Try to connect to Central
		if d.isCentralReady() {
			d.logger.Success("✓ Central is ready and responding!")
			return true
		}

		// Report progress periodically
		if time.Since(lastProgressReport) >= progressInterval {
			elapsed := time.Since(deadline.Add(-timeout))
			remaining := timeout - elapsed
			d.logger.Dim(fmt.Sprintf("  ⋯ Still waiting for Central... (%v elapsed, %v remaining)",
				elapsed.Round(time.Second), remaining.Round(time.Second)))
			lastProgressReport = time.Now()
		}

		time.Sleep(checkInterval)
	}

	d.logger.Warning("⚠️  Central did not become ready within the timeout period")
	d.logger.Warning("   This is not necessarily an error - Central may still be initializing")
	d.logger.Warning("   You can check Central status manually or wait a bit longer")
	return false
}

// isCentralReady checks if Central is responding to HTTP requests
func (d *Deployer) isCentralReady() bool {
	// Use exec to run curl with a short timeout
	// We use -k to skip TLS verification
	endpoint := fmt.Sprintf("https://%s", d.centralEndpoint)
	cmd := exec.Command("curl", "-k", "-s", "-o", "/dev/null", "-w", "%{http_code}",
		"--connect-timeout", "3", "--max-time", "5", endpoint)

	output, _ := cmd.Output()
	// Even if curl exits with error, we might have gotten a status code
	statusCode := strings.TrimSpace(string(output))

	if len(statusCode) == 0 {
		return false
	}

	// Central returns 200 for the UI root, or possibly 401/403 if auth is required
	// We consider any successful HTTP response (2xx, 3xx, 4xx) as "ready"
	// Only connection failures (empty response or network errors) mean "not ready"
	firstChar := statusCode[0]
	return firstChar >= '2' && firstChar <= '4'
}

// cleanupTempDir safely removes a temporary directory with logging
func (d *Deployer) cleanupTempDir(path string, description string) {
	if path == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		d.logger.Warningf("Failed to cleanup %s at %s: %v", description, path, err)
	} else {
		d.logger.Dim(fmt.Sprintf("Cleaned up %s: %s", description, path))
	}
}

func (d *Deployer) writeEnvrcFile(ctx context.Context, exposure string, portForwardWanted bool) error {
	endpoint := strings.TrimPrefix(d.centralEndpoint, "https://")

	content := fmt.Sprintf(`export API_ENDPOINT="%s"
export ROX_ENDPOINT="%s"
export ROX_BASE_URL="https://%s"
export ROX_ADMIN_PASSWORD="%s"
export ROX_CA_CERT_FILE="%s"
`, endpoint, endpoint, endpoint, d.centralPassword, d.roxCACertFile)

	if err := os.WriteFile(d.envrcFile, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write envrc file: %w", err)
	}

	d.logger.Successf("✓ Environment variables written to %s", d.envrcFile)
	return nil
}

func (d *Deployer) PrintCentralDeploymentSummary() {
	component := "Central"
	mainImageTag := d.mainImageTag
	helm := d.useHelm
	olm := d.useOLM
	exposure := d.exposure
	portForwarding := d.portForwardEnabled
	log := d.logger
	kubeContext := d.kubeContext

	// Calculate box width
	boxWidth := 60

	// Helper function to truncate long values
	truncate := func(s string, maxLen int) string {
		if len(s) <= maxLen {
			return s
		}
		return s[:maxLen-3] + "..."
	}

	// Helper function to create a row with alignment on colon
	createRow := func(label, value string) string {
		// Fixed label width for alignment (adjust this to fit longest label)
		labelWidth := 22
		// Maximum value length to keep total under boxWidth
		maxValueLen := boxWidth - labelWidth - 4 // 4 = space + colon + space + border
		truncatedValue := truncate(value, maxValueLen)

		// Right-align label, then colon, then value
		labelPadding := labelWidth - len(label)
		if labelPadding < 0 {
			labelPadding = 0
		}
		content := fmt.Sprintf(" %s%s: %s", strings.Repeat(" ", labelPadding), label, truncatedValue)

		// Pad to box width
		padding := boxWidth - len(content) - 1
		if padding < 0 {
			padding = 0
		}
		return content + strings.Repeat(" ", padding) + " │"
	}

	// Print the box
	cyan := color.New(color.FgCyan, color.Bold)

	log.Info("")
	log.Info(cyan.Sprint("┌" + strings.Repeat("─", boxWidth) + "┐"))

	title := " Deployment Configuration"
	titlePadding := boxWidth - len(title)
	log.Info(cyan.Sprint("│") + title + strings.Repeat(" ", titlePadding) + cyan.Sprint("│"))
	log.Info(cyan.Sprint("├" + strings.Repeat("─", boxWidth) + "┤"))

	// Deployment details
	log.Info(cyan.Sprint("│") + createRow("Component", component))
	log.Info(cyan.Sprint("│") + createRow("Cluster Type", env.GetCurrentClusterType().String()))
	log.Info(cyan.Sprint("│") + createRow("Main Tag", mainImageTag))
	log.Info(cyan.Sprint("│") + createRow("Kubernetes Context", kubeContext))
	log.Info(cyan.Sprint("│") + createRow("Deployment Method", map[bool]string{true: "Helm", false: "Operator"}[helm]))

	if olm {
		log.Info(cyan.Sprint("│") + createRow("OLM", "Yes"))
	}

	log.Info(cyan.Sprint("│") + createRow("Exposure", exposure))

	if portForwarding || exposure == "none" {
		log.Info(cyan.Sprint("│") + createRow("Port Forwarding", "Enabled (localhost:8443)"))
	}

	log.Info(cyan.Sprint("└" + strings.Repeat("─", boxWidth) + "┘"))
	log.Info("")
}

// checkDeploymentProgressInNamespace checks for deployment state changes in a specific namespace and reports them
func (d *Deployer) checkDeploymentProgressInNamespace(ctx context.Context, namespace string, seenDeployments map[string]string) {
	result, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{"get", "deployments", "-n", namespace, "-o", "jsonpath={range .items[*]}{.metadata.name}{'|'}{.status.replicas}{'|'}{.status.readyReplicas}{'|'}{.status.availableReplicas}{'\\n'}{end}"},
	})
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		name := parts[0]
		replicas := parts[1]
		ready := parts[2]
		available := parts[3]

		// Create state key
		stateKey := fmt.Sprintf("%s:%s:%s:%s", name, replicas, ready, available)

		// Check if this is a new deployment or state change
		if prevState, exists := seenDeployments[name]; !exists {
			// New deployment detected
			d.logger.Dimf("  → Deployment '%s' created (%s/%s replicas ready)", name, ready, replicas)
			seenDeployments[name] = stateKey
		} else if prevState != stateKey {
			// State changed
			if available != "" && available != "0" && available == replicas {
				d.logger.Dimf("  ✓ Deployment '%s' is available (%s/%s replicas)", name, available, replicas)
			} else if ready != prevState[len(name)+1:] {
				d.logger.Dimf("  ⋯ Deployment '%s' progressing (%s/%s replicas ready)", name, ready, replicas)
			}
			seenDeployments[name] = stateKey
		}
	}
}

// checkPodProgressInNamespace checks for pod state changes in a specific namespace and reports them
func (d *Deployer) checkPodProgressInNamespace(ctx context.Context, namespace string, seenPods map[string]string) {
	result, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{"get", "pods", "-n", namespace, "-o", "jsonpath={range .items[*]}{.metadata.name}{'|'}{.status.phase}{'|'}{.status.containerStatuses[0].ready}{'\\n'}{end}"},
	})
	if err != nil {
		return
	}

	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		name := parts[0]
		phase := parts[1]
		ready := ""
		if len(parts) > 2 {
			ready = parts[2]
		}

		stateKey := fmt.Sprintf("%s:%s", phase, ready)

		// Only report significant state changes
		if prevState, exists := seenPods[name]; !exists {
			if phase == "Pending" {
				d.logger.Dim(fmt.Sprintf("    • Pod '%s' starting...", name))
			} else if phase == "Running" && ready == "true" {
				d.logger.Dim(fmt.Sprintf("    • Pod '%s' running", name))
			}
			seenPods[name] = stateKey
		} else if prevState != stateKey {
			if phase == "Running" && ready == "true" {
				d.logger.Dim(fmt.Sprintf("    • Pod '%s' is ready", name))
			} else if phase == "Running" && ready == "false" {
				d.logger.Dim(fmt.Sprintf("    • Pod '%s' running (not ready yet)", name))
			}
			seenPods[name] = stateKey
		}
	}
}

func (d *Deployer) PrintSecuredClusterDeploymentSummary() {
	component := "Secured Cluster"
	mainImageTag := d.mainImageTag
	helm := d.useHelm
	olm := d.useOLM
	log := d.logger
	kubeContext := d.kubeContext

	// Calculate box width
	boxWidth := 60

	// Helper function to truncate long values
	truncate := func(s string, maxLen int) string {
		if len(s) <= maxLen {
			return s
		}
		return s[:maxLen-3] + "..."
	}

	// Helper function to create a row with alignment on colon
	createRow := func(label, value string) string {
		// Fixed label width for alignment (adjust this to fit longest label)
		labelWidth := 22
		// Maximum value length to keep total under boxWidth
		maxValueLen := boxWidth - labelWidth - 4 // 4 = space + colon + space + border
		truncatedValue := truncate(value, maxValueLen)

		// Right-align label, then colon, then value
		labelPadding := labelWidth - len(label)
		if labelPadding < 0 {
			labelPadding = 0
		}
		content := fmt.Sprintf(" %s%s: %s", strings.Repeat(" ", labelPadding), label, truncatedValue)

		// Pad to box width
		padding := boxWidth - len(content) - 1
		if padding < 0 {
			padding = 0
		}
		return content + strings.Repeat(" ", padding) + " │"
	}

	// Print the box
	cyan := color.New(color.FgCyan, color.Bold)

	log.Info("")
	log.Info(cyan.Sprint("┌" + strings.Repeat("─", boxWidth) + "┐"))

	title := " Deployment Configuration"
	titlePadding := boxWidth - len(title)
	log.Info(cyan.Sprint("│") + title + strings.Repeat(" ", titlePadding) + cyan.Sprint("│"))
	log.Info(cyan.Sprint("├" + strings.Repeat("─", boxWidth) + "┤"))

	// Deployment details
	log.Info(cyan.Sprint("│") + createRow("Component", component))
	log.Info(cyan.Sprint("│") + createRow("Cluster Type", env.GetCurrentClusterType().String()))
	log.Info(cyan.Sprint("│") + createRow("Main Tag", mainImageTag))
	log.Info(cyan.Sprint("│") + createRow("Kubernetes Context", kubeContext))
	log.Info(cyan.Sprint("│") + createRow("Deployment Method", map[bool]string{true: "Helm", false: "Operator"}[helm]))

	if olm {
		log.Info(cyan.Sprint("│") + createRow("OLM", "Yes"))
	}

	log.Info(cyan.Sprint("└" + strings.Repeat("─", boxWidth) + "┘"))
	log.Info("")
}
