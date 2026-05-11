package deployer

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"

	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/dockerauth"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/imagecache"
	"github.com/stackrox/roxie/internal/k8s"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/portforward"
	"github.com/stackrox/roxie/internal/types"
)

var (
	DefaultCentralWaitTimeout        = 20 * time.Minute
	DefaultSecuredClusterWaitTimeout = 20 * time.Minute

	pauseReconcileAnnotationKey = "stackrox.io/pause-reconcile"

	// AdminUsername is the default admin username for StackRox Central
	AdminUsername = "admin"

	// TODO(#91): at some point this will get out of date. If we filter by the app.../part-of
	// label anyway, then maybe we should just delete all resource kinds present on cluster?
	// also we should use the fully-qualified types
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

	injectedCABundleConfigMapPrefix         = "injected-cabundle-"
	injectedCABundleConfigMapCentral        = injectedCABundleConfigMapPrefix + centralCrName
	injectedCABundleConfigMapSecuredCluster = injectedCABundleConfigMapPrefix + securedClusterCrName
)

// Deployer is the base deployer for ACS
type Deployer struct {
	// Influencing roxies mode of operation.
	verbose     bool
	logger      *logger.Logger
	startTime   time.Time
	dockerAuth  *dockerauth.DockerAuth
	imageCache  *imagecache.ImageCache
	dockerCreds *dockerauth.Credentials
	envrcFile   string

	kubeContext          string
	clusterResourceKinds map[string]struct{}

	config Config

	// State
	centralEndpoint string
	centralPassword string
	roxCACertFile   string
	tempDir         string
	portForward     *portforward.Manager
}

type ResourceToDelete struct {
	Kind      string
	Name      string
	OwnerName string
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
	return d.deleteResources(ctx, namespace, []string{resourceType}, append([]string{resourceName}, args...)...)
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
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{Args: finalArgs})
	return err
}

func (d *Deployer) deleteFinalizers(ctx context.Context, namespace, resourceType, resourceName string) error {
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{
			"-n", namespace, "patch", resourceType, resourceName,
			"-p", `{"metadata":{"finalizers":null}}`,
			"--type=merge",
		},
	})
	return err
}

// Expects that reconciliation for the RHACS operator is paused.
func (d *Deployer) deleteCentralResources(ctx context.Context, wait bool) error {
	d.logger.Info("Deleting Central resources")
	var crExists bool

	if d.doesResourceExist(ctx, "central", "stackrox-central-services", d.config.Central.Namespace) {
		crExists = true

		// Trigger async deletion of the Central CR.
		err := d.deleteResource(ctx, d.config.Central.Namespace, "central", "stackrox-central-services", "--wait=false")
		if err != nil {
			return fmt.Errorf("failed to asynchronously delete Central CR: %w", err)
		}

		err = d.deleteFinalizers(ctx, d.config.Central.Namespace, "central", "stackrox-central-services")
		if err != nil {
			return fmt.Errorf("failed to delete finalizers on Central CR: %w", err)
		}
	}

	// Pause reconciliation for other controllers, not just our RHACS operator.
	// This is needed to ensure that there is no race causing the Cluster Network Operator
	// to re-create the injected-ca-bundle ConfigMap during resource deletion.
	if err := d.preventOtherControllersFromReconciling(ctx, component.Central); err != nil {
		return fmt.Errorf("failed to prevent other controllers from reconciling Central resources: %w", err)
	}

	// Delete other resources by brute force.
	resourceKinds := d.filterResourceKinds(allInstallableCentralResourceKinds)
	err := d.deleteResources(ctx, d.config.Central.Namespace, resourceKinds, "-l=app.kubernetes.io/part-of=stackrox-central-services")
	if err != nil {
		return err
	}

	for _, resource := range []ResourceToDelete{
		{Name: "central-db", Kind: "pvc", OwnerName: centralCrName},
		{Name: "central-db-backup", Kind: "pvc", OwnerName: centralCrName},
		{Name: "admin-password", Kind: "secret"},
		{Name: "scanner-db-password", Kind: "secret", OwnerName: centralCrName},
		// In case the Cluster Network Operator has succeeded in re-creating the injected-cabundle configmap
		// after our operator has already deleted it.
		{Name: injectedCABundleConfigMapCentral, Kind: "configmap"},
	} {
		d.logger.Dimf("Attempting to delete %s/%s", resource.Kind, resource.Name)
		if resource.OwnerName != "" {
			// Avoid deletion if the resource does not have the expected owner.
			// (e.g. in case central and secured cluster are deployed into the same namespace).
			obj, err := k8s.RetrieveResourceFromCluster(ctx, d.logger, d.config.Central.Namespace, resource.Kind, resource.Name)
			if err != nil {
				if !k8s.IsResourceNotFound(err) {
					d.logger.Warningf("Failed to retrieve %s/%s for owner checking: %v. Skipping deletion. Deployment might be affected.", resource.Kind, resource.Name, err)
				}
				continue
			}
			if k8s.ResourceNotOwnedByName(obj, resource.OwnerName) {
				d.logger.Dimf("Skipping deletion of %s/%s: not owned by %s", resource.Kind, resource.Name, resource.OwnerName)
				continue
			}
		}

		if err := d.deleteResource(ctx, d.config.Central.Namespace, resource.Kind, resource.Name); err != nil {
			return fmt.Errorf("failed to delete %s/%s: %w", resource.Kind, resource.Name, err)
		}
	}

	if crExists {
		// Now delete the Central CR synchronously.
		err := d.deleteResource(ctx, d.config.Central.Namespace, "central", "stackrox-central-services")
		if err != nil {
			return fmt.Errorf("failed to delete Central CR: %w", err)
		}
	}

	return nil
}

func (d *Deployer) preventOtherControllersFromReconciling(ctx context.Context, comp component.Component) error {
	switch comp {
	case component.Central:
		return d.preventCABundleInjection(ctx, injectedCABundleConfigMapCentral, d.config.Central.Namespace)
	case component.SecuredCluster:
		return d.preventCABundleInjection(ctx, injectedCABundleConfigMapSecuredCluster, d.config.SecuredCluster.Namespace)
	default:
		return nil
	}
}

func (d *Deployer) preventCABundleInjection(ctx context.Context, configMapName, namespace string) error {
	d.logger.Info("Removing CNO label from injected-cabundle ConfigMap to prevent CNO from injecting the CA bundle during cleanup")
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{
			"label", "configmap", configMapName, "-n", namespace,
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

	if d.doesResourceExist(ctx, "securedcluster", "stackrox-secured-cluster-services", d.config.SecuredCluster.Namespace) {
		crExists = true

		// Trigger async deletion of the SecuredCluster CR.
		err := d.deleteResource(ctx, d.config.SecuredCluster.Namespace, "securedcluster", "stackrox-secured-cluster-services", "--wait=false")
		if err != nil {
			return err
		}

		err = d.deleteFinalizers(ctx, d.config.SecuredCluster.Namespace, "securedcluster", "stackrox-secured-cluster-services")
		if err != nil {
			return fmt.Errorf("failed to delete finalizers on SecuredCluster CR: %w", err)
		}
	}

	// Pause reconciliation for other controllers, not just our RHACS operator.
	// This is needed to ensure that there is no race causing the Cluster Network Operator
	// to re-create the injected-ca-bundle ConfigMap during resource deletion.
	if err := d.preventOtherControllersFromReconciling(ctx, component.SecuredCluster); err != nil {
		return fmt.Errorf("failed to prevent other controllers from reconciling SecuredCluster resources: %w", err)
	}

	// In the meantime, delete other resources by brute force.
	resourceKinds := d.filterResourceKinds(allInstallableSecuredClusterResourceKinds)
	err := d.deleteResources(ctx, d.config.SecuredCluster.Namespace, resourceKinds, "-l=app.kubernetes.io/part-of=stackrox-secured-cluster-services")
	if err != nil {
		return err
	}

	for _, resource := range []ResourceToDelete{
		{Name: "cluster-registration-secret", Kind: "secret"},
		// We need to make sure that don't accidentally delete a scanner-db-password belonging to the central CR,
		// when both are deployed into the same namespace.
		{Name: "scanner-db-password", Kind: "secret", OwnerName: securedClusterCrName},
		// In case the Cluster Network Operator has succeeded in re-creating the injected-cabundle configmap
		// after our operator has already deleted it.
		{Name: injectedCABundleConfigMapSecuredCluster, Kind: "configmap"},
	} {
		d.logger.Dimf("Attempting to delete %s/%s", resource.Kind, resource.Name)
		if resource.OwnerName != "" {
			// Avoid deletion if the resource does not have the expected owner.
			// (e.g. in case central and secured cluster are deployed into the same namespace).
			obj, err := k8s.RetrieveResourceFromCluster(ctx, d.logger, d.config.SecuredCluster.Namespace, resource.Kind, resource.Name)
			if err != nil {
				if !k8s.IsResourceNotFound(err) {
					d.logger.Warningf("Failed to retrieve %s/%s for owner checking: %v. Skipping deletion. Deployment might be affected.", resource.Kind, resource.Name, err)
				}
				continue
			}
			if k8s.ResourceNotOwnedByName(obj, resource.OwnerName) {
				d.logger.Dimf("Skipping deletion of %s/%s: not owned by %s", resource.Kind, resource.Name, resource.OwnerName)
				continue
			}
		}
		if err := d.deleteResource(ctx, d.config.SecuredCluster.Namespace, resource.Kind, resource.Name); err != nil {
			return fmt.Errorf("failed to delete %s/%s: %w", resource.Kind, resource.Name, err)
		}
	}

	if crExists {
		// Now delete the SecuredCluster CR synchronously.
		err := d.deleteResource(ctx, d.config.SecuredCluster.Namespace, "securedcluster", "stackrox-secured-cluster-services")
		if err != nil {
			return fmt.Errorf("failed to delete SecuredCluster CR: %w", err)
		}
	}

	return nil
}

func (d *Deployer) SetConfig(config Config) {
	d.config = config
}

// New creates a new Deployer instance.
// It verifies that the current environment contains necessary tools.
// It creates a temporary directory for the deployer to use during deployment,
// and it is the caller's responsibility to clean it up using the Cleanup() method when not used anymore.
func New(log *logger.Logger) (*Deployer, error) {
	if err := checkRequiredTools(); err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "roxie-deployer-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	d := &Deployer{
		logger:    log,
		startTime: time.Now(),
		tempDir:   tempDir,
	}

	d.dockerAuth = dockerauth.New(log)
	d.imageCache = imagecache.New(log, "", 20)
	d.portForward = portforward.New(k8s.GetKubectl(), log)

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

	d.kubeContext = env.GetCurrentContext()

	if d.kubeContext != "" {
		clusterResourceKinds, err := d.getClusterResourceKinds()
		if err != nil {
			return nil, fmt.Errorf("failed to get cluster resource kinds: %w", err)
		}
		d.clusterResourceKinds = clusterResourceKinds
	}

	log.Success("🚀 ACS Deployer initialized")

	return d, nil
}

func (d *Deployer) getClusterResourceKinds() (map[string]struct{}, error) {
	result, err := d.runKubectl(context.Background(), k8s.KubectlOptions{
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

// Cleanup cleans up any temporary resources created by the deployer, such as temporary files.
func (d *Deployer) Cleanup() {
	if d.tempDir != "" && d.envrcFile == "" {
		// In the case of envrc file usage, we need to keep temporary files around after deployment.
		// (It contains CA certificates, for example.)
		if err := os.RemoveAll(d.tempDir); err != nil {
			d.logger.Warningf("Deployer Cleanup failed to remove %q: %v", d.tempDir, err)
		}
	}
}

// Deploy deploys the specified components to the cluster.
func (d *Deployer) Deploy(ctx context.Context, components component.Component) error {
	// Prepare and verify credentials early to fail fast.
	if env.GetCurrentClusterType() != types.ClusterTypeInfraOpenShift4 {
		if err := d.prepareCredentials(); err != nil {
			return fmt.Errorf("failed to prepare credentials: %w", err)
		}
	}

	d.logger.Infof("Initiating deployment of %s", components)

	// If only deploying operator, use the operator-only flow.
	if components.IncludesOperatorExplicitly() {
		return d.deployOperatorOnly(ctx)
	}

	// Deploy operator first if needed.
	if err := d.ensureOperatorDeployed(ctx); err != nil {
		return fmt.Errorf("failed to deploy operator: %w", err)
	}

	if components.IncludesCentral() {
		if err := d.deployCentral(ctx); err != nil {
			return fmt.Errorf("failed to deploy central: %w", err)
		}
	}
	if components.IncludesSensor() {
		if err := d.deploySecuredCluster(ctx); err != nil {
			return fmt.Errorf("failed to deploy secured cluster: %w", err)
		}
	}
	return nil
}

// prepareCredentials prepares and verifies Docker credentials early to allow failing fast.
// The verified credentials are stored in the Deployer object for later use.
func (d *Deployer) prepareCredentials() error {
	d.logger.Dimf("Preparing and verifying Docker credentials...")

	// This will retrieve and verify credentials, returning error if invalid
	creds, err := d.dockerAuth.GetAndVerifyCredentials()
	if err != nil {
		return err
	}

	d.dockerCreds = creds

	d.logger.Dimf("Docker credentials verified successfully")
	return nil
}

func (d *Deployer) deployCentral(ctx context.Context) error {
	d.logger.Infof("Deploying Central to namespace %s", d.config.Central.Namespace)
	if d.namespaceExists(d.config.Central.Namespace) {
		d.logger.Info("Existing Central deployment found, tearing down...")
		if err := d.teardownCentral(ctx); err != nil {
			d.logger.Warningf("Error during teardown: %v", err)
		}
	}

	if err := d.deployCentralOperator(ctx); err != nil {
		return err
	}

	// envrc may be used from different processes, so use actual endpoint not port-forward
	if d.envrcFile != "" {
		d.logger.Dimf("Writing environment variables to %s", d.envrcFile)
		if err := d.writeEnvrcFile(ctx); err != nil {
			d.logger.Warningf("Failed to write envrc file: %v", err)
		}
	}

	return nil
}

func (d *Deployer) deploySecuredCluster(ctx context.Context) error {
	d.logger.Infof("Deploying SecuredCluster to namespace %s", d.config.SecuredCluster.Namespace)
	if d.namespaceExists(d.config.SecuredCluster.Namespace) {
		d.logger.Info("Existing SecuredCluster deployment found, tearing down...")
		if err := d.teardownSecuredCluster(ctx); err != nil {
			d.logger.Warningf("Error during teardown: %v", err)
		}
	}

	return d.deploySecuredClusterOperator(ctx)
}

func (d *Deployer) Teardown(ctx context.Context, components component.Component) error {
	d.logger.Infof("Starting teardown of %s", components)

	switch components {
	case component.Central:
		return d.teardownCentral(ctx)
	case component.SecuredCluster:
		return d.teardownSecuredCluster(ctx)
	case component.Both, component.All:
		// Tear down components in parallel for better performance
		var wg sync.WaitGroup

		// Always tear down central and sensor
		wg.Add(2)

		go func() {
			defer wg.Done()
			if err := d.teardownSecuredCluster(ctx); err != nil {
				d.logger.Warningf("Error tearing down secured cluster: %v", err)
			}
		}()

		go func() {
			defer wg.Done()
			if err := d.teardownCentral(ctx); err != nil {
				d.logger.Warningf("Error tearing down central: %v", err)
			}
		}()

		wg.Wait()

		// Tear down the operator strictly after Central/SecuredCluster are gone,
		// because the operator manages finalizers on their custom resources.
		if components == component.All {
			if err := d.teardownOperator(ctx); err != nil {
				d.logger.Warningf("Error tearing down operator: %v", err)
			}
		}

		return nil
	default:
		return fmt.Errorf("unknown component: %s", components)
	}
}

func (d *Deployer) teardownCentral(ctx context.Context) error {
	d.logger.Infof("🗑️  Tearing down central in namespace %s", d.config.Central.Namespace)

	if !d.namespaceExists(d.config.Central.Namespace) {
		d.logger.Infof("Namespace %s doesn't exist, skipping", d.config.Central.Namespace)
		return nil
	}

	d.portForward.Stop()

	// Add pause-reconcile annotation to not have the operator interfere during resource deletion.
	if d.doesResourceExist(ctx, "central", "stackrox-central-services", d.config.Central.Namespace) {
		if err := d.addPauseReconcileAnnotation(ctx, "central", "stackrox-central-services", d.config.Central.Namespace); err != nil {
			d.logger.Warningf("Error adding pause-reconcile annotation: %v", err)
		}
	}

	d.logger.Info("⏳ Waiting for Central resources to be fully deleted...")
	err := d.deleteCentralResources(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to delete Central resources: %w", err)
	}

	d.logger.Successf("✓ Central resources in namespace %s have been deleted", d.config.Central.Namespace)
	return nil
}

func (d *Deployer) teardownSecuredCluster(ctx context.Context) error {
	d.logger.Infof("🗑️  Tearing down secured cluster in namespace %s", d.config.SecuredCluster.Namespace)

	if !d.namespaceExists(d.config.SecuredCluster.Namespace) {
		d.logger.Infof("Namespace %s doesn't exist, skipping", d.config.SecuredCluster.Namespace)
		return nil
	}

	if d.doesResourceExist(ctx, "securedcluster", "stackrox-secured-cluster-services", d.config.SecuredCluster.Namespace) {
		// Add pause-reconcile annotation to not have the operator interfere during resource deletion.
		if err := d.addPauseReconcileAnnotation(ctx, "securedcluster", "stackrox-secured-cluster-services", d.config.SecuredCluster.Namespace); err != nil {
			d.logger.Warningf("Error adding pause-reconcile annotation: %v", err)
		}
	}

	d.logger.Info("⏳ Waiting for SecuredCluster resources to be fully deleted...")
	err := d.deleteSecuredClusterResources(ctx, true)
	if err != nil {
		return fmt.Errorf("failed to delete SecuredCluster resources: %w", err)
	}

	d.logger.Successf("✓ SecuredCluster resources in namespace %s have been deleted", d.config.SecuredCluster.Namespace)
	return nil
}

func (d *Deployer) ensureNamespaceExists(namespace string) error {
	if d.namespaceExists(namespace) {
		return nil
	}

	d.logger.Infof("Creating namespace %s", namespace)
	_, err := d.runKubectl(context.Background(), k8s.KubectlOptions{
		Args: []string{"create", "namespace", namespace},
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Label namespace as managed by roxie since we just created it
	_, err = d.runKubectl(context.Background(), k8s.KubectlOptions{
		Args: []string{"label", "namespace", namespace,
			"app.kubernetes.io/managed-by=roxie", "--overwrite"},
	})
	if err != nil {
		d.logger.Warningf("failed to label namespace %s: %v", namespace, err)
	}

	return nil
}

func (d *Deployer) namespaceExists(namespace string) bool {
	_, err := d.runKubectl(context.Background(), k8s.KubectlOptions{
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

	if len(missing) > 0 {
		return fmt.Errorf("required tools not found in PATH: %s\nPlease install these tools and ensure they are available in your PATH", strings.Join(missing, ", "))
	}

	return nil
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

func (d *Deployer) SetVerbose(verbose bool) {
	d.verbose = verbose
}

func (d *Deployer) doesResourceExist(ctx context.Context, resourceType, resourceName, namespace string) bool {
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{
			"get", resourceType, resourceName,
			"-n", namespace,
		},
	})
	return err == nil
}

func (d *Deployer) addPauseReconcileAnnotation(ctx context.Context, resourceType, resourceName, namespace string) error {
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
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

// WaitForCentral waits for Central to be ready and responding on its endpoint
// Returns true if Central is ready, false if timeout occurs
func (d *Deployer) WaitForCentral(timeout time.Duration) bool {
	if d.centralEndpoint == "" {
		d.logger.Dim("No Central endpoint configured, skipping readiness check")
		return false
	}

	if env.RunningInteractively {
		d.logger.Infof("⏳ Waiting for Central to be ready at %s (timeout: %v)", d.centralEndpoint, timeout)
	} else {
		d.logger.Infof("⏳ Waiting for Central to be ready (timeout: %v)", timeout)
	}

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

func (d *Deployer) writeEnvrcFile(ctx context.Context) error {
	var content strings.Builder
	fmt.Fprintf(&content, "export API_ENDPOINT=%q\n", d.centralEndpoint)
	fmt.Fprintf(&content, "export ROX_ENDPOINT=%q\n", d.centralEndpoint)
	fmt.Fprintf(&content, "export ROX_BASE_URL='https://%s'\n", d.centralEndpoint)
	fmt.Fprintf(&content, "export ROX_USERNAME=%q\n", AdminUsername)
	fmt.Fprintf(&content, "export ROX_ADMIN_PASSWORD=%q\n", d.centralPassword)
	fmt.Fprintf(&content, "export ROX_CA_CERT_FILE=%q\n", d.roxCACertFile)

	if err := os.WriteFile(d.envrcFile, []byte(content.String()), 0600); err != nil {
		return fmt.Errorf("failed to write envrc file: %w", err)
	}

	d.logger.Successf("✓ Environment variables written to %s", d.envrcFile)
	return nil
}

func (d *Deployer) PrintCentralDeploymentSummary() {
	component := "Central"
	mainImageTag := d.config.Roxie.Version
	olm := d.config.Operator.DeployViaOlm
	exposure := d.config.Central.GetExposure()
	portForwarding := d.config.Central.PortForwardingEnabled()
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

	if olm {
		log.Info(cyan.Sprint("│") + createRow("OLM", "Yes"))
	}

	log.Info(cyan.Sprint("│") + createRow("Exposure", exposure.String()))

	if portForwarding || exposure == types.ExposureNone {
		log.Info(cyan.Sprint("│") + createRow("Port Forwarding", "Enabled (localhost:8443)"))
	}

	log.Info(cyan.Sprint("└" + strings.Repeat("─", boxWidth) + "┘"))
	log.Info("")
}

// checkDeploymentProgressInNamespace checks for deployment state changes in a specific namespace and reports them
func (d *Deployer) checkDeploymentProgressInNamespace(ctx context.Context, namespace string, seenDeployments map[string]string) {
	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
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
	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
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

// TODO(#91): plenty of code in common with the central variant that should probably be
// extracted
func (d *Deployer) PrintSecuredClusterDeploymentSummary() {
	component := "Secured Cluster"
	mainImageTag := d.config.Roxie.Version
	olm := d.config.Operator.DeployViaOlm
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

	if olm {
		log.Info(cyan.Sprint("│") + createRow("OLM", "Yes"))
	}

	log.Info(cyan.Sprint("└" + strings.Repeat("─", boxWidth) + "┘"))
	log.Info("")
}

type CentralDeploymentInfo struct {
	Endpoint       string
	Password       string
	KubeContext    string
	Exposure       types.Exposure
	CACertFile     string
	HAProxyStarted bool
}

func (d *Deployer) GetCentralDeploymentInfo() CentralDeploymentInfo {
	return CentralDeploymentInfo{
		Endpoint:    d.centralEndpoint,
		Password:    d.centralPassword,
		KubeContext: d.kubeContext,
		Exposure:    d.config.Central.GetExposure(),
		CACertFile:  d.roxCACertFile,
	}
}
