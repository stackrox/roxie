package deployer

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/stackrox/roxie/internal/component"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/k8s"
	log "github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/types"
)

const (
	loadBalancerWaitTime = 10 * time.Minute
)

var (
	smallScale = map[string]interface{}{
		"autoScaling": "Enabled",
		"replicas":    1,
	}
	noScaling = map[string]interface{}{
		"autoScaling": "Disabled",
		"replicas":    1,
	}
)

// deployOperatorOnly deploys only the operator without any Central or SecuredCluster resources
func (d *Deployer) deployOperatorOnly(ctx context.Context) error {
	log.Info("🚀 Deploying Operator only...")

	if err := d.ensureOperatorDeployed(ctx); err != nil {
		return err
	}

	log.Success("✓ Operator deployed successfully")
	log.Info("You can now deploy Central or SecuredCluster components separately")
	return nil
}

// ensureOperatorDeployed ensures the required operator instance(s) are deployed.
func (d *Deployer) ensureOperatorDeployed(ctx context.Context) error {
	// Skip operator deployment/checks if flag is set to false
	if d.config.Operator.SkipDeploymentEnabled() {
		log.Info("ℹ️  Skipping operator deployment checks (--deploy-operator=false)")
		log.Info("   Assuming operator is already running...")
		return nil
	}

	if err := d.ensureCRDsInstalled(ctx); err != nil {
		return fmt.Errorf("failed to ensure CRDs installed: %w", err)
	}

	instances, olmMode, err := d.config.OperatorInstances()
	if err != nil {
		return err
	}

	if err := d.teardownStaleOperatorNamespaces(ctx, instances); err != nil {
		return err
	}

	for i := range instances {
		populateOperatorEnvVars(&instances[i])
	}

	if olmMode {
		return d.ensureOperatorDeployedOLM(ctx, instances[0])
	}

	// Non-OLM case.

	operatorExists, currentMode, err := d.detectOperatorDeploymentMode(ctx)
	if err != nil {
		return fmt.Errorf("detecting operator deployment mode: %w", err)
	}
	if operatorExists && currentMode == OperatorModeOLM {
		log.Info("🔄 Switching operator from OLM to non-OLM mode...")
		if err := d.teardownOperatorOLM(ctx); err != nil {
			return fmt.Errorf("failed to teardown OLM operator: %w", err)
		}
	}

	for _, instance := range instances {
		if err := d.ensureOperatorInstanceNonOLM(ctx, instance); err != nil {
			return fmt.Errorf("failed to deploy operator in namespace %s: %w", instance.Namespace, err)
		}
	}

	return nil
}

// teardownStaleOperatorNamespaces removes operators from namespaces that are no longer
// part of the desired deployment plan (e.g. switching between single and mixed versions).
func (d *Deployer) teardownStaleOperatorNamespaces(ctx context.Context, desired []OperatorInstanceConfig) error {
	desiredNS := make(map[string]bool, len(desired))
	for _, inst := range desired {
		desiredNS[inst.Namespace] = true
	}

	for _, ns := range AllOperatorNamespaces {
		if desiredNS[ns] {
			continue
		}
		if !d.operatorDeploymentExists(ctx, ns) && !d.namespaceExists(ns) {
			continue
		}
		log.Infof("🔄 Removing previous operator from namespace %s (no longer needed)...", ns)
		instance := OperatorInstanceConfig{Namespace: ns}
		switch ns {
		case operatorNamespaceCentral:
			instance.RoleNameSuffix = roleNameSuffixCentral
		case operatorNamespaceSensor:
			instance.RoleNameSuffix = roleNameSuffixSensor
		}
		if err := d.teardownOperatorNonOLMInNamespace(ctx, instance); err != nil {
			return fmt.Errorf("tearing down operator in namespace %s: %w", ns, err)
		}
	}
	return nil
}

// ensureOperatorInstanceNonOLM ensures one non-OLM operator instance matches the desired version.
func (d *Deployer) ensureOperatorInstanceNonOLM(ctx context.Context, instance OperatorInstanceConfig) error {
	exists := d.operatorDeploymentExists(ctx, instance.Namespace)
	needsDeployment := !exists
	needsTeardown := false

	if exists {
		if d.isOperatorImageCorrect(ctx, instance) {
			log.Infof("✓ Operator already deployed with correct image in namespace %s", instance.Namespace)
			return nil
		}
		log.Infof("🔄 Operator image mismatch in namespace %s, redeploying...", instance.Namespace)
		needsTeardown = true
		needsDeployment = true
	}

	if needsTeardown {
		if err := d.teardownOperatorNonOLMInNamespace(ctx, instance); err != nil {
			return fmt.Errorf("failed to teardown non-OLM operator: %w", err)
		}
	}

	if needsDeployment {
		if err := d.deployOperatorNonOLM(ctx, instance); err != nil {
			return fmt.Errorf("failed to deploy operator: %w", err)
		}
	}

	return nil
}

// ensureOperatorDeployedOLM is the single-operator OLM path (mixed versions are rejected in validation).
func (d *Deployer) ensureOperatorDeployedOLM(ctx context.Context, instance OperatorInstanceConfig) error {
	operatorExists, currentMode, err := d.detectOperatorDeploymentMode(ctx)
	if err != nil {
		return fmt.Errorf("detecting operator deployment mode: %w", err)
	}
	needsDeployment := false
	needsTeardown := false

	if !operatorExists {
		needsDeployment = true
	} else if currentMode == OperatorModeNonOLM {
		log.Info("🔄 Switching operator from non-OLM to OLM mode...")
		needsTeardown = true
		needsDeployment = true
	} else {
		if d.isOperatorImageCorrect(ctx, instance) {
			log.Info("✓ Operator already deployed with correct image")
			// TODO(#304): Changes in the operator environment configuration do not
			// trigger a redeployment of the operator.
		} else {
			log.Info("🔄 Operator image mismatch, redeploying...")
			needsTeardown = true
			needsDeployment = true
		}
	}

	if needsTeardown {
		if currentMode == OperatorModeOLM {
			if err := d.teardownOperatorOLM(ctx); err != nil {
				return fmt.Errorf("failed to teardown OLM operator: %w", err)
			}
		} else {
			if err := d.teardownOperatorNonOLM(ctx); err != nil {
				return fmt.Errorf("failed to teardown non-OLM operator: %w", err)
			}
		}
	}

	if needsDeployment {
		if err := d.deployOperatorViaOLM(ctx, instance); err != nil {
			return fmt.Errorf("failed to deploy operator via OLM: %w", err)
		}
	}

	return nil
}

// deployCentralOperator deploys Central using the operator
func (d *Deployer) deployCentralOperator(ctx context.Context) error {
	log.Info("🚀 Deploying Central via Operator...")

	needPullSecrets := d.NeedsPullSecrets(ctx)
	if err := d.prepareNamespace(ctx, d.config.Central.Namespace, needPullSecrets); err != nil {
		return fmt.Errorf("failed to prepare namespace: %w", err)
	}

	if err := d.createAdminPasswordSecret(ctx); err != nil {
		return fmt.Errorf("failed to create admin password secret: %w", err)
	}

	cr, err := d.config.Central.CustomResource()
	if err != nil {
		return fmt.Errorf("failed to build Central CR: %w", err)
	}

	if err := d.applyCentralCR(ctx, cr); err != nil {
		return fmt.Errorf("failed to apply Central CR: %w", err)
	}

	if err := d.waitForComponentReady(ctx, component.Central); err != nil {
		return fmt.Errorf("failed waiting for Central: %w", err)
	}

	if d.config.Central.PauseReconciliationEnabled() {
		log.Infof("Adding pause-reconcile annotation to Central")
		err := d.addPauseReconcileAnnotation(ctx, "Central", centralCrName, d.config.Central.Namespace)
		if err != nil {
			return err
		}
	}

	return d.configureCentralEndpoint(ctx)
}

// isOperatorImageCorrect checks if the deployed operator matches the desired
// image, comparing the full reference (registry, repository, and tag).
func (d *Deployer) isOperatorImageCorrect(ctx context.Context, instance OperatorInstanceConfig) bool {
	currentImage, err := d.getDeployedOperatorImage(ctx, instance.Namespace)
	if err != nil {
		log.Warningf("Could not retrieve operator image: %v", err)
		return false
	}

	desiredImage := instance.OperatorImage()
	if currentImage != desiredImage {
		log.Info("Operator image mismatch detected:")
		log.Infof("  Current: %s", currentImage)
		log.Infof("  Desired: %s", desiredImage)
		return false
	}
	return true
}

// getDeployedOperatorImage gets the image of the currently deployed operator in a namespace.
func (d *Deployer) getDeployedOperatorImage(ctx context.Context, namespace string) (string, error) {
	result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "deployment", operatorDeploymentName, "-n", namespace,
			"-o", "jsonpath={.spec.template.spec.containers[0].image}"},
	})
	if err != nil {
		return "", err
	}

	image := strings.TrimSpace(result.Stdout)
	return image, nil
}

// prepareNamespace creates pull secrets in the namespace if needed
func (d *Deployer) prepareNamespace(ctx context.Context, namespace string, needPullSecrets bool) error {
	log.Infof("Preparing namespace %s", namespace)

	if err := d.ensureNamespaceExists(namespace); err != nil {
		return err
	}

	if needPullSecrets {
		if err := d.ensurePullSecretExists(ctx, namespace); err != nil {
			return fmt.Errorf("ensuring image pull secret exists: %w", err)
		}
	}

	return nil
}

func (d *Deployer) ensurePullSecretExists(ctx context.Context, namespace string) error {
	if d.dockerCreds == nil {
		return errors.New("no pull secrets available to set up on the cluster")
	}

	pullSecretYAML := d.dockerAuth.CreatePullSecretYAMLFromCredentials(*d.dockerCreds, namespace, d.config.Roxie.ImageRegistry)
	_, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: strings.NewReader(pullSecretYAML),
	})
	if err != nil {
		log.Warningf("Could not apply pull secret: %v", err)
	}

	return nil
}

// createAdminPasswordSecret creates the admin password secret
func (d *Deployer) createAdminPasswordSecret(ctx context.Context) error {
	secret := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      adminPasswordSecretName,
			"namespace": d.config.Central.Namespace,
		},
		"type": "Opaque",
		"stringData": map[string]string{
			"password": d.centralPassword,
		},
	}

	yamlData, err := yaml.Marshal(secret)
	if err != nil {
		return fmt.Errorf("failed to marshal secret: %w", err)
	}

	result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		stderr := strings.TrimSpace(result.Stderr)
		if len(stderr) > 0 {
			log.Errorf("kubectl apply produced error output:")
			for line := range strings.SplitSeq(stderr, "\n") {
				log.Dim("| " + line)
			}
		}
		return err
	}

	log.Success("✓ Admin password secret created")
	return nil
}

func getCentralResourcesOperator(resourcesProfile types.ResourceProfile) map[string]interface{} {
	switch resourcesProfile {
	case types.ResourceProfileTiny:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"central": map[string]interface{}{
					"resources": centralResourcesTiny,
					"db": map[string]interface{}{
						"resources": centralDbResourcesTiny,
						"persistence": map[string]interface{}{
							"persistentVolumeClaim": map[string]interface{}{
								"size": centralDbPVCSizeTiny,
							},
						},
					},
				},
				"scanner": map[string]interface{}{
					"scannerComponent": "Disabled",
				},
				"scannerV4": map[string]interface{}{
					"db": map[string]interface{}{
						"resources": centralScannerV4DbResourcesTiny,
					},
					"indexer": map[string]interface{}{
						"resources": centralScannerV4IndexerResourcesTiny,
						"scaling":   noScaling,
					},
					"matcher": map[string]interface{}{
						"resources": centralScannerV4MatcherResourcesTiny,
						"scaling":   noScaling,
					},
				},
			},
		}
	case types.ResourceProfileSmall:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"central": map[string]interface{}{
					"resources": centralResourcesSmall,
					"db": map[string]interface{}{
						"resources": centralDbResourcesSmall,
						"persistence": map[string]interface{}{
							"persistentVolumeClaim": map[string]interface{}{
								"size": centralDbPVCSizeSmall,
							},
						},
					},
				},
				"scanner": map[string]interface{}{
					"scannerComponent": "Disabled",
					"analyzer": map[string]interface{}{
						"scaling":   noScaling,
						"resources": centralScannerResourcesSmall,
					},
					"db": map[string]interface{}{
						"resources": centralScannerDbResourcesSmall,
					},
				},
				"scannerV4": map[string]interface{}{
					"db": map[string]interface{}{
						"resources": centralScannerV4DbResourcesSmall,
					},
					"indexer": map[string]interface{}{
						"resources": centralScannerV4IndexerResourcesSmall,
						"scaling":   noScaling,
					},
					"matcher": map[string]interface{}{
						"resources": centralScannerV4MatcherResourcesSmall,
						"scaling":   noScaling,
					},
				},
			},
		}
	case types.ResourceProfileMedium:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"central": map[string]interface{}{
					"resources": centralResourcesMedium,
					"db": map[string]interface{}{
						"resources": centralDbResourcesMedium,
					},
				},
				"scanner": map[string]interface{}{
					"analyzer": map[string]interface{}{
						"scaling":   noScaling,
						"resources": centralScannerResourcesMedium,
					},
					"db": map[string]interface{}{
						"resources": centralScannerDbResourcesMedium,
					},
				},
				"scannerV4": map[string]interface{}{
					"db": map[string]interface{}{
						"resources": centralScannerV4DbResourcesMedium,
					},
					"indexer": map[string]interface{}{
						"resources": centralScannerV4IndexerResourcesMedium,
						"scaling":   noScaling,
					},
					"matcher": map[string]interface{}{
						"resources": centralScannerV4MatcherResourcesMedium,
						"scaling":   noScaling,
					},
				},
			},
		}
	case types.ResourceProfileCI:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"central": map[string]interface{}{
					"db": map[string]interface{}{
						"resources": centralDbResourcesCI,
					},
				},
				"scanner": map[string]interface{}{
					"analyzer": map[string]interface{}{
						"scaling": noScaling,
					},
				},
				"scannerV4": map[string]interface{}{
					"db": map[string]interface{}{
						"resources": centralScannerV4DbResourcesCI,
					},
					"indexer": map[string]interface{}{
						"resources": centralScannerV4IndexerResourcesCI,
						"scaling":   noScaling,
					},
					"matcher": map[string]interface{}{
						"resources": centralScannerV4MatcherResourcesCI,
						"scaling":   noScaling,
					},
				},
			},
		}
	default:
		return nil
	}
}

// applyCentralCR applies the Central CR to the cluster
func (d *Deployer) applyCentralCR(ctx context.Context, cr map[string]interface{}) error {
	log.Info("Applying Central custom resource")

	if env.RunningInteractively {
		log.Debug("Central CR YAML:")
		log.DebugMultilineYaml(cr)
	} else {
		log.Debug("Skipping emitting Central CR in non-interactive mode, because it could leak confidential information")
	}

	yamlData, err := yaml.Marshal(cr)
	if err != nil {
		return fmt.Errorf("failed to marshal Central CR: %w", err)
	}

	result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		log.Errorf("kubectl stdout: %s", result.Stdout)
		log.Errorf("kubectl stderr: %s", result.Stderr)
		return fmt.Errorf("failed to apply Central CR: %w\nStderr: %s", err, result.Stderr)
	}

	log.Success("✓ Central Custom Resource applied")
	return nil
}

// waitForAvailableCondition waits for the specified namespace/resource
func (d *Deployer) waitForAvailableCondition(ctx context.Context, resource, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// We need this extra step, because a "kubectl wait" fails immediately, when waiting for readiness of a deployment
	// which does not exist (yet).
	if err := d.waitForResourceToExist(ctx, resource, namespace); err != nil {
		return fmt.Errorf("error waiting for resource %s in namespace %s to exist: %v", resource, namespace, err)
	}

	result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args: []string{
			"wait",
			"--for=condition=Available",
			resource,
			"-n", namespace,
			"--timeout=" + time.Until(deadline).String(),
		},
	})
	if err != nil {
		stderr := strings.TrimSpace(result.Stderr)
		if len(stderr) > 0 {
			log.Errorf("kubectl wait produced error output:")
			for line := range strings.SplitSeq(stderr, "\n") {
				log.Dim("| " + line)
			}
		}
		return fmt.Errorf("error waiting for resource %s in namespace %s to become Available: %v", resource, namespace, err)
	}
	log.Successf("✓ Resource %s in namespace %s is ready", resource, namespace)
	return nil
}

// waitForResourceToExist polls until the given resource exists in the namespace.
func (d *Deployer) waitForResourceToExist(ctx context.Context, resource, namespace string) error {
	log.Infof("Waiting for resource %s to exist in namespace %s...", resource, namespace)
	for {
		_, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", resource, "-n", namespace},
		})
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (d *Deployer) getWaitConfig(comp component.Component) (WaitConfig, error) {
	switch comp {
	case component.Central:
		return d.config.Central.GetWaitConfig(), nil
	case component.SecuredCluster:
		return d.config.SecuredCluster.GetWaitConfig(), nil
	default:
		return WaitConfig{}, fmt.Errorf("unsupported component for wait config: %s", comp)
	}
}

// waitForComponentReady waits for a component to be ready.
func (d *Deployer) waitForComponentReady(ctx context.Context, comp component.Component) error {
	waitCfg, err := d.getWaitConfig(comp)
	if err != nil {
		return err
	}
	log.Infof("⏳ Waiting for %s to become ready (timeout: %s)...", comp, waitCfg.Timeout)

	const padding = 5 * time.Second
	waitCtx, cancel := context.WithTimeout(ctx, waitCfg.Timeout+padding)
	defer cancel()

	// Spawn a goroutine, which waits until some success condition, sending the result (nil or error) through a dedicated channel.
	waitChannel := make(chan error)

	go func() {
		err := d.waitForAvailableCondition(waitCtx, waitCfg.WaitFor, waitCfg.Namespace, waitCfg.Timeout)
		if err != nil {
			waitChannel <- fmt.Errorf("error waiting for %s deployment to become Available: %v", comp, err)
			return
		}
		log.Infof("Resource %s is now ready.", waitCfg.WaitFor)
		waitChannel <- nil
	}()

	// Show progress while waiting for the wait-goroutine above to terminate.
	seenDeployments := make(map[string]string)
	seenPods := make(map[string]string)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	// We will log a generic message when we haven't seen pod/deployment progress
	// for longer than this duration.
	progressUpdatePeriod := 1 * time.Minute
	lastUpdate := time.Now()

	for {
		select {
		case err := <-waitChannel:
			return err
		case <-waitCtx.Done():
			return fmt.Errorf("timeout reached")
		case <-ticker.C:
			// Track seen deployments and their states to avoid duplicate messages.
			deploymentsProgressed, err := d.checkDeploymentProgressInNamespace(waitCtx, waitCfg.Namespace, seenDeployments)
			if err != nil {
				log.Warningf("failed to check for deployment progress in namespace %s: %v", waitCfg.Namespace, err)
			}
			podsProgressed, err := d.checkPodProgressInNamespace(waitCtx, waitCfg.Namespace, seenPods)
			if err != nil {
				log.Warningf("failed to check for pod progress in namespace %s: %v", waitCfg.Namespace, err)
			}
			if deploymentsProgressed || podsProgressed {
				lastUpdate = time.Now()
			} else {
				if time.Since(lastUpdate) > progressUpdatePeriod {
					log.Dimf("Still waiting for component %s in namespace %s", comp, waitCfg.Namespace)
					lastUpdate = time.Now()
				}
			}
		}
	}
}

// waitForLoadBalancer waits for a LoadBalancer service to get an external IP.
// Returns the endpoint as "host:port", with no https:// prefix.
func (d *Deployer) waitForLoadBalancer(ctx context.Context, namespace, serviceName string, waitTime time.Duration) (string, error) {
	log.Infof("⏳ Waiting %s for LoadBalancer %s to get external IP...", waitTime, serviceName)

	start := time.Now()
	for time.Since(start) < waitTime {
		result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "svc", serviceName, "-n", namespace, "-o", "jsonpath={.status.loadBalancer.ingress[0].ip}"},
		})
		if err == nil && result.Stdout != "" {
			ip := strings.TrimSpace(result.Stdout)
			if ip != "" && ip != "<pending>" {
				if env.RunningInteractively {
					log.Successf("✓ LoadBalancer IP: %s", ip)
				} else {
					log.Success("✓ LoadBalancer IP")
				}
				return fmt.Sprintf("%s:443", ip), nil
			}
		}

		// Also check for hostname (some cloud providers use hostname instead of IP)
		result, err = k8s.RunKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "svc", serviceName, "-n", namespace, "-o", "jsonpath={.status.loadBalancer.ingress[0].hostname}"},
		})
		if err == nil && result.Stdout != "" {
			hostname := strings.TrimSpace(result.Stdout)
			if hostname != "" && hostname != "<pending>" {
				if env.RunningInteractively {
					log.Successf("✓ LoadBalancer hostname: %s", hostname)
				} else {
					log.Success("✓ LoadBalancer hostname")
				}
				return fmt.Sprintf("%s:443", hostname), nil
			}
		}

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return "", fmt.Errorf("cancelled waiting for LoadBalancer to get external address: %w", ctx.Err())
		}
	}

	return "", errors.New("timeout waiting for LoadBalancer to get external address")
}

// routeStatus is the subset of the OpenShift Route object waitForRoute needs.
type routeStatus struct {
	Spec struct {
		Host string `json:"host"`
	} `json:"spec"`
	Status struct {
		Ingress []struct {
			Host       string `json:"host"`
			Conditions []struct {
				Type   string `json:"type"`
				Status string `json:"status"`
			} `json:"conditions"`
		} `json:"ingress"`
	} `json:"status"`
}

// fetchRouteStatus reads the Route's host and admission state from a single
// object, so both reflect the same state of the Route. Admission conditions
// live on the per-router status.ingress entries; a host conflict can leave the
// Route unadmitted with a host set, so admission is only reported when an
// ingress entry for the host carries Admitted=True.
func fetchRouteStatus(ctx context.Context, namespace, routeName string) (host string, admitted bool, err error) {
	result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "route", routeName, "-n", namespace, "-o", "json"},
	})
	if err != nil {
		return "", false, err
	}
	var route routeStatus
	if err := json.Unmarshal([]byte(result.Stdout), &route); err != nil {
		return "", false, err
	}
	host, admitted = routeAdmittedFor(route)
	return host, admitted, nil
}

// routeAdmittedFor reports the Route's host and whether an ingress entry for
// that host carries an Admitted=True condition.
func routeAdmittedFor(route routeStatus) (string, bool) {
	for _, ingress := range route.Status.Ingress {
		if ingress.Host != route.Spec.Host {
			continue
		}
		for _, condition := range ingress.Conditions {
			if condition.Type == "Admitted" && condition.Status == "True" {
				return route.Spec.Host, true
			}
		}
	}
	return route.Spec.Host, false
}

// waitForRoute waits for an OpenShift Route to be admitted by the router.
// Returns the endpoint as "host:port", with no https:// prefix.
func (d *Deployer) waitForRoute(ctx context.Context, namespace, routeName string, waitTime time.Duration) (string, error) {
	log.Infof("⏳ Waiting %s for Route %s to be admitted...", waitTime, routeName)

	start := time.Now()
	for time.Since(start) < waitTime {
		hostname, admitted, err := fetchRouteStatus(ctx, namespace, routeName)
		if err == nil && hostname != "" && admitted {
			if env.RunningInteractively {
				log.Successf("✓ Route admitted with hostname: %s", hostname)
			} else {
				log.Success("✓ Route admitted")
			}
			return fmt.Sprintf("%s:443", hostname), nil
		}

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return "", fmt.Errorf("cancelled waiting for Route to be admitted: %w", ctx.Err())
		}
	}

	return "", errors.New("timeout waiting for Route to be admitted")
}

// fetchCentralCACerts fetches CA certificates needed to verify Central's TLS cert.
// It always fetches the internal StackRox CA from the central-tls secret, and
// additionally fetches CA certs from the defaultTLSSecret if one is configured
// in the Central CR spec.
func (d *Deployer) fetchCentralCACerts(ctx context.Context) error {
	var caPEMs [][]byte

	// Fetch the internal StackRox CA from the central-tls secret.
	log.Info("Fetching internal CA certificate from central-tls secret...")
	internalCA, err := d.fetchSecretField(ctx, "central-tls", "ca\\.pem")
	if err != nil {
		return fmt.Errorf("failed to get CA cert from central-tls secret: %w", err)
	}
	caPEMs = append(caPEMs, internalCA)

	// If a custom defaultTLSSecret is configured, add its certs to the
	// trust pool. This includes the leaf — Central itself does the same
	// when building the trust bundle for Sensor.
	if secretName := d.defaultTLSSecretName(); secretName != "" {
		log.Infof("Fetching custom TLS certificates from secret %s...", secretName)
		customCerts, err := d.fetchCustomTLSCerts(ctx, secretName)
		if err != nil {
			log.Warningf("Could not fetch custom TLS certs from secret %s: %v", secretName, err)
			// Try to continue.
		} else {
			caPEMs = append(caPEMs, customCerts...)
		}
	}

	caCertFile, err := os.CreateTemp(d.tempDir, "roxie-central-ca-*.pem")
	if err != nil {
		return fmt.Errorf("failed to create temp file for CA cert: %w", err)
	}

	fileName := caCertFile.Name()
	for _, pemData := range caPEMs {
		pemData = append(bytes.TrimRight(pemData, "\n"), '\n')
		if _, err := caCertFile.Write(pemData); err != nil {
			_ = caCertFile.Close()
			_ = os.Remove(fileName)
			return fmt.Errorf("failed to write CA cert: %w", err)
		}
	}
	if err := caCertFile.Close(); err != nil {
		_ = os.Remove(fileName)
		return fmt.Errorf("failed to close CA cert file: %w", err)
	}

	d.roxCACertFile = fileName
	log.Successf("✓ CA certificates saved to: %s", d.roxCACertFile)
	return nil
}

// defaultTLSSecretName returns the name of the defaultTLSSecret from the Central
// CR spec, or "" if none is configured.
func (d *Deployer) defaultTLSSecretName() string {
	name, _, _ := unstructured.NestedString(d.config.Central.Spec, "central", "defaultTLSSecret", "name")
	return name
}

// fetchSecretField fetches a single base64-encoded field from a Kubernetes secret
// in the Central namespace and returns the decoded PEM bytes.
func (d *Deployer) fetchSecretField(ctx context.Context, secretName, jsonpathField string) ([]byte, error) {
	result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "secret", secretName, "-n", d.config.Central.Namespace, "-o", fmt.Sprintf("jsonpath={.data.%s}", jsonpathField)},
	})
	if err != nil {
		return nil, fmt.Errorf("kubectl get secret %s: %w", secretName, err)
	}

	b64 := strings.TrimSpace(result.Stdout)
	if b64 == "" {
		return nil, fmt.Errorf("field %s in secret %s is empty", jsonpathField, secretName)
	}

	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("base64 decode of %s from %s: %w", jsonpathField, secretName, err)
	}
	return decoded, nil
}

// fetchCustomTLSCerts fetches the tls.crt field from a Kubernetes TLS secret
// and returns all certificates (leaf, intermediates, root) as PEM blocks.
// All certs are added to the trust pool, matching how Central itself builds
// the trust bundle for Sensor.
func (d *Deployer) fetchCustomTLSCerts(ctx context.Context, secretName string) ([][]byte, error) {
	certBundle, err := d.fetchSecretField(ctx, secretName, "tls\\.crt")
	if err != nil {
		return nil, fmt.Errorf("fetching certificates from secret %s: %w", secretName, err)
	}

	var pems [][]byte
	for block, rest := pem.Decode(certBundle); block != nil; block, rest = pem.Decode(rest) {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parsing certificate from secret %s: %w", secretName, err)
		}
		log.Infof("Found certificate in %s: Subject.CN=%q, IsCA=%v",
			secretName, cert.Subject.CommonName, cert.IsCA)
		pems = append(pems, pem.EncodeToMemory(block))
	}
	return pems, nil
}

// configureCentralEndpoint configures the central endpoint in the Deployer based on exposure settings.
func (d *Deployer) configureCentralEndpoint(ctx context.Context) error {
	exposure := d.config.Central.GetExposure()
	if d.config.Central.PortForwardingEnabled() {
		// Start port-forward for CLI tool access via localhost:8443
		serviceName := "central"
		if exposure == types.ExposureLoadBalancer {
			_, err := d.waitForLoadBalancer(ctx, d.config.Central.Namespace, "central-loadbalancer", loadBalancerWaitTime)
			if err != nil {
				log.Warningf("LoadBalancer not ready: %v", err)
			} else {
				serviceName = "central-loadbalancer"
			}
		}

		if d.config.Roxie.EnvrcSet() {
			endpoint, pid, err := d.portForward.StartDetached(d.config.Central.Namespace, serviceName, 443, 8443)
			if err != nil {
				return fmt.Errorf("failed to start detached port-forward: %w", err)
			}
			d.centralEndpoint = endpoint
			d.portForwardPID = pid
		} else {
			endpoint, err := d.portForward.Start(d.config.Central.Namespace, serviceName, 443, 8443)
			if err != nil {
				return fmt.Errorf("failed to start port-forward: %w", err)
			}
			d.centralEndpoint = endpoint
		}
	} else if exposure == types.ExposureLoadBalancer {
		endpoint, err := d.waitForLoadBalancer(ctx, d.config.Central.Namespace, "central-loadbalancer", loadBalancerWaitTime)
		if err != nil {
			return fmt.Errorf("failed to get LoadBalancer endpoint: %w", err)
		}
		d.centralEndpoint = endpoint
	} else if exposure == types.ExposureRoute {
		endpoint, err := d.waitForRoute(ctx, d.config.Central.Namespace, "central", loadBalancerWaitTime)
		if err != nil {
			return fmt.Errorf("failed to get Route endpoint: %w", err)
		}
		d.centralEndpoint = endpoint
	} else {
		d.centralEndpoint = "central." + d.config.Central.Namespace + ".svc:443"
	}

	if err := d.fetchCentralCACerts(ctx); err != nil {
		log.Warningf("Could not fetch Central CA certs: %v", err)
	}

	if env.RunningInteractively {
		log.Successf("✓ Central is ready at: %s", d.centralEndpoint)
		log.Successf("✓ Admin password: %s", d.centralPassword)
	}

	return nil
}

// deploySecuredClusterOperator deploys SecuredCluster using the operator.
func (d *Deployer) deploySecuredClusterOperator(ctx context.Context) error {
	log.Info("🚀 Deploying SecuredCluster via Operator...")

	needPullSecrets := d.NeedsPullSecrets(ctx)
	if err := d.prepareNamespace(ctx, d.config.SecuredCluster.Namespace, needPullSecrets); err != nil {
		return fmt.Errorf("failed to prepare namespace: %w", err)
	}

	cr, err := d.config.SecuredCluster.CustomResource()
	if err != nil {
		return fmt.Errorf("failed to build SecuredCluster CR: %w", err)
	}

	clusterName, found, err := unstructured.NestedString(cr, "spec", "clusterName")
	if err != nil {
		return fmt.Errorf("failed to get cluster name from SecuredCluster CR: %w", err)
	}
	if !found || clusterName == "" {
		return fmt.Errorf("cluster name not found in SecuredCluster CR")
	}
	log.Infof("Using cluster name: %s", clusterName)

	crsContent, err := d.generateCRS(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("failed to generate CRS: %w", err)
	}

	if err := d.applyCRS(ctx, crsContent); err != nil {
		return fmt.Errorf("failed to apply CRS: %w", err)
	}

	if err := d.applySecuredClusterCR(ctx, cr); err != nil {
		return fmt.Errorf("failed to apply SecuredCluster CR: %w", err)
	}

	if err := d.waitForComponentReady(ctx, component.SecuredCluster); err != nil {
		return fmt.Errorf("failed waiting for SecuredCluster: %w", err)
	}

	if d.config.SecuredCluster.PauseReconciliationEnabled() {
		log.Infof("Adding pause-reconcile annotation to SecuredCluster")
		err := d.addPauseReconcileAnnotation(ctx, "SecuredCluster", securedClusterCrName, d.config.SecuredCluster.Namespace)
		if err != nil {
			return err
		}
	}

	log.Successf("✓ SecuredCluster '%s' is ready", clusterName)
	return nil
}

func getSecuredClusterResourcesOperator(resourceProfile types.ResourceProfile) map[string]interface{} {
	switch resourceProfile {
	case types.ResourceProfileTiny:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"admissionControl": map[string]interface{}{
					"replicas": 1,
				},
				"sensor": map[string]interface{}{
					"resources": securedClusterSensorResourcesTiny,
				},
				"scanner": map[string]interface{}{
					"scannerComponent": "Disabled",
				},
				"scannerV4": map[string]interface{}{
					"scannerComponent": "Disabled",
				},
				// The CRS init container's resource requests are not exposed in the CR spec,
				// so we use the operator's overlay mechanism to lower them for tiny clusters.
				"overlays": []any{
					map[string]any{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"name":       "sensor",
						"optional":   true,
						"patches": []any{
							map[string]any{
								"path":  "spec.template.spec.initContainers[name:crs].resources.requests",
								"value": `{"cpu":"80m", "memory": "150Mi"}`,
							},
						},
					},
				},
			},
		}
	case types.ResourceProfileSmall:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"admissionControl": map[string]interface{}{
					"replicas": 1,
				},
				"sensor": map[string]interface{}{
					"resources": securedClusterSensorResourcesSmall,
				},
				"scanner": map[string]interface{}{
					"scannerComponent": "Disabled",
					"analyzer": map[string]interface{}{
						"scaling": smallScale,
					},
				},
				"scannerV4": map[string]interface{}{
					"indexer": map[string]interface{}{
						"scaling": noScaling,
					},
				},
			},
		}
	case types.ResourceProfileMedium:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"admissionControl": map[string]interface{}{
					"replicas": 1,
				},
				"scanner": map[string]interface{}{
					"analyzer": map[string]interface{}{
						"scaling": smallScale,
					},
				},
				"sensor": map[string]interface{}{
					"resources": securedClusterSensorResourcesMedium,
				},
				"scannerV4": map[string]interface{}{
					"indexer": map[string]interface{}{
						"scaling": noScaling,
					},
				},
			},
		}
	case types.ResourceProfileCI:
		return map[string]interface{}{
			"spec": map[string]interface{}{
				"sensor": map[string]interface{}{
					"resources": securedClusterSensorResourcesCI,
				},
			},
		}
	default:
		return nil
	}
}

// applySecuredClusterCR applies the SecuredCluster CR to the cluster
func (d *Deployer) applySecuredClusterCR(ctx context.Context, cr map[string]interface{}) error {
	log.Info("Applying SecuredCluster custom resource")

	if env.RunningInteractively {
		log.Debug("SecuredCluster CR YAML:")
		log.DebugMultilineYaml(cr)
	} else {
		log.Debug("Skipping emitting SecuredCluster CR in non-interactive mode, because it could leak confidential information")
	}

	yamlData, err := yaml.Marshal(cr)
	if err != nil {
		return fmt.Errorf("failed to marshal SecuredCluster CR: %w", err)
	}

	result, err := k8s.RunKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-n", d.config.SecuredCluster.Namespace, "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		log.Errorf("kubectl error: %s", result.Stderr)
		return fmt.Errorf("failed to apply SecuredCluster CR: %w", err)
	}

	log.Success("✓ SecuredCluster CR applied")
	return nil
}
