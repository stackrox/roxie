package deployer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/k8s"
	"github.com/stackrox/roxie/internal/types"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	d.logger.Info("🚀 Deploying Operator only...")

	if err := d.ensureOperatorDeployed(ctx); err != nil {
		return err
	}

	d.logger.Success("✓ Operator deployed successfully")
	d.logger.Info("You can now deploy Central or SecuredCluster components separately")
	return nil
}

// ensureOperatorDeployed ensures the operator is deployed with the correct version and mode
func (d *Deployer) ensureOperatorDeployed(ctx context.Context) error {
	// Skip operator deployment/checks if flag is set to false
	if d.config.Operator.SkipDeployment {
		d.logger.Info("ℹ️  Skipping operator deployment checks (--deploy-operator=false)")
		d.logger.Info("   Assuming operator is already running...")
		return nil
	}

	if err := d.ensureCRDsInstalled(ctx); err != nil {
		return fmt.Errorf("failed to ensure CRDs installed: %w", err)
	}

	// Detect current operator deployment mode
	operatorExists, currentMode, err := d.detectOperatorDeploymentMode(ctx)
	if err != nil {
		return fmt.Errorf("detecting operator deployment mode: %w", err)
	}
	needsDeployment := false
	needsTeardown := false

	if !operatorExists {
		needsDeployment = true
	} else if d.config.Operator.DeployViaOlm && currentMode == OperatorModeNonOLM {
		// Switching from non-OLM to OLM
		d.logger.Info("🔄 Switching operator from non-OLM to OLM mode...")
		needsTeardown = true
		needsDeployment = true
	} else if !d.config.Operator.DeployViaOlm && currentMode == OperatorModeOLM {
		// Switching from OLM to non-OLM
		d.logger.Info("🔄 Switching operator from OLM to non-OLM mode...")
		needsTeardown = true
		needsDeployment = true
	} else {
		// Same mode, check version
		if d.isOperatorVersionCorrect(ctx) {
			d.logger.Info("✓ Operator already deployed with correct version")
		} else {
			d.logger.Info("🔄 Operator version mismatch, redeploying...")
			needsTeardown = true
			needsDeployment = true
		}
	}

	if needsTeardown {
		// Perform teardown for the current mode
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
		if d.config.Operator.DeployViaOlm {
			if err := d.deployOperatorViaOLM(ctx); err != nil {
				return fmt.Errorf("failed to deploy operator via OLM: %w", err)
			}
		} else {
			if err := d.deployOperatorNonOLM(ctx); err != nil {
				return fmt.Errorf("failed to deploy operator: %w", err)
			}
		}
	}

	return nil
}

// deployCentralOperator deploys Central using the operator
func (d *Deployer) deployCentralOperator(ctx context.Context) error {
	d.logger.Info("🚀 Deploying Central via Operator...")

	needPullSecrets := env.GetCurrentClusterType() != types.ClusterTypeInfraOpenShift4
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

	if err := d.waitForCentralReady(ctx); err != nil {
		return fmt.Errorf("failed waiting for Central: %w", err)
	}

	if d.config.Central.PauseReconciliation {
		d.logger.Infof("Adding pause-reconcile annotation to Central")
		err := d.addPauseReconcileAnnotation(ctx, "Central", centralCrName, d.config.Central.Namespace)
		if err != nil {
			return err
		}
	}

	return d.configureCentralEndpoint(ctx)
}

// isOperatorVersionCorrect checks if the deployed operator matches the desired version
func (d *Deployer) isOperatorVersionCorrect(ctx context.Context) bool {
	currentImage, err := d.getDeployedOperatorImage(ctx)
	if err != nil {
		d.logger.Warningf("Could not retrieve operator image: %v", err)
		return false
	}

	// Extract the tag from the current image
	parts := strings.SplitN(currentImage, ":", 2)
	if len(parts) < 2 {
		d.logger.Warningf("Could not parse operator image tag from: %s", currentImage)
		return false
	}
	currentTag := parts[1]

	if currentTag != d.config.Operator.Version {
		d.logger.Info("Operator version mismatch detected:")
		d.logger.Infof("  Current: %s", currentTag)
		d.logger.Infof("  Desired: %s", d.config.Operator.Version)
		return false
	}
	return true
}

// getDeployedOperatorImage gets the image of the currently deployed operator
func (d *Deployer) getDeployedOperatorImage(ctx context.Context) (string, error) {
	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "deployment", operatorDeploymentName, "-n", operatorNamespace,
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
	d.logger.Infof("Preparing namespace %s", namespace)

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

	pullSecretYAML := d.dockerAuth.CreatePullSecretYAMLFromCredentials(*d.dockerCreds, namespace)
	_, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: strings.NewReader(pullSecretYAML),
	})
	if err != nil {
		d.logger.Warningf("Could not apply pull secret: %v", err)
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

	_, err = d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create admin password secret: %w", err)
	}

	d.logger.Success("✓ Admin password secret created")
	return nil
}

func getCentralResourcesOperator(resourcesProfile types.ResourceProfile) map[string]interface{} {
	switch resourcesProfile {
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
	d.logger.Info("Applying Central custom resource")

	if d.verbose {
		if env.RunningInteractively {
			d.logger.Dim("Central CR YAML:")
			helpers.LogMultilineYaml(d.logger, cr)
		} else {
			d.logger.Dim("Skipping emitting Central CR in non-interactive mode, because it could leak confidential information")
		}
	}

	yamlData, err := yaml.Marshal(cr)
	if err != nil {
		return fmt.Errorf("failed to marshal Central CR: %w", err)
	}

	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		d.logger.Errorf("kubectl stdout: %s", result.Stdout)
		d.logger.Errorf("kubectl stderr: %s", result.Stderr)
		return fmt.Errorf("failed to apply Central CR: %w\nStderr: %s", err, result.Stderr)
	}

	d.logger.Success("✓ Central Custom Resource applied")
	return nil
}

// waitForCentralReady waits for Central to be ready
func (d *Deployer) waitForCentralReady(ctx context.Context) error {
	timeout := d.config.Central.DeployTimeout
	d.logger.Infof("⏳ Waiting for Central to become ready (timeout: %s)...", timeout)

	// Track seen deployments and their states to avoid duplicate messages
	seenDeployments := make(map[string]string)
	seenPods := make(map[string]string)

	start := time.Now()
	checkInterval := 3 * time.Second

	for time.Since(start) < timeout {
		// Check for new deployments
		d.checkDeploymentProgress(ctx, seenDeployments)

		// Check for pod events if in early readiness mode or verbose
		if d.config.Central.EarlyReadiness || d.verbose {
			d.checkPodProgress(ctx, seenPods)
		}

		// Check if central deployment is ready
		result, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "deployment", "central", "-n", d.config.Central.Namespace, "-o", "jsonpath={.status.readyReplicas}"},
		})
		if err == nil && result.Stdout != "" {
			replicas := strings.TrimSpace(result.Stdout)
			if replicas != "0" && replicas != "" {
				d.logger.Successf("✓ Central is ready (%s replicas)", replicas)
				return nil
			}
		}

		// TODO(ROX-34499): using `kubectl wait` (which in turn - I hope - uses a watch) instead of
		// polling would allow us to not waste time here
		time.Sleep(checkInterval)
	}

	return errors.New("timeout waiting for Central to become ready")
}

// checkDeploymentProgress checks for deployment state changes and reports them
func (d *Deployer) checkDeploymentProgress(ctx context.Context, seenDeployments map[string]string) {
	d.checkDeploymentProgressInNamespace(ctx, d.config.Central.Namespace, seenDeployments)
}

// checkPodProgress checks for pod state changes and reports them
func (d *Deployer) checkPodProgress(ctx context.Context, seenPods map[string]string) {
	d.checkPodProgressInNamespace(ctx, d.config.Central.Namespace, seenPods)
}

// waitForLoadBalancer waits for a LoadBalancer service to get an external IP.
// Returns the endpoint as "host:port", with no https:// prefix.
func (d *Deployer) waitForLoadBalancer(ctx context.Context, namespace, serviceName string, timeout int) (string, error) {
	d.logger.Infof("⏳ Waiting for LoadBalancer %s to get external IP...", serviceName)

	start := time.Now()
	for time.Since(start) < time.Duration(timeout)*time.Second {
		result, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "svc", serviceName, "-n", namespace, "-o", "jsonpath={.status.loadBalancer.ingress[0].ip}"},
		})
		if err == nil && result.Stdout != "" {
			ip := strings.TrimSpace(result.Stdout)
			if ip != "" && ip != "<pending>" {
				if env.RunningInteractively {
					d.logger.Successf("✓ LoadBalancer IP: %s", ip)
				} else {
					d.logger.Success("✓ LoadBalancer IP")
				}
				return fmt.Sprintf("%s:443", ip), nil
			}
		}

		// Also check for hostname (some cloud providers use hostname instead of IP)
		result, err = d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "svc", serviceName, "-n", namespace, "-o", "jsonpath={.status.loadBalancer.ingress[0].hostname}"},
		})
		if err == nil && result.Stdout != "" {
			hostname := strings.TrimSpace(result.Stdout)
			if hostname != "" && hostname != "<pending>" {
				if env.RunningInteractively {
					d.logger.Successf("✓ LoadBalancer hostname: %s", hostname)
				} else {
					d.logger.Success("✓ LoadBalancer hostname")
				}
				return fmt.Sprintf("%s:443", hostname), nil
			}
		}

		time.Sleep(5 * time.Second)
	}

	return "", errors.New("timeout waiting for LoadBalancer to get external address")
}

// fetchCentralCACert fetches the Central CA certificate
func (d *Deployer) fetchCentralCACert(ctx context.Context) error {
	d.logger.Info("Fetching Central CA certificate...")

	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args: []string{"get", "secret", "central-tls", "-n", d.config.Central.Namespace, "-o", "jsonpath={.data.ca\\.pem}"},
	})
	if err != nil {
		return fmt.Errorf("failed to get CA cert from secret: %w", err)
	}

	caCertBase64 := strings.TrimSpace(result.Stdout)
	if caCertBase64 == "" {
		return errors.New("CA certificate is empty")
	}

	decodeCmd := exec.CommandContext(ctx, "base64", "-d")
	decodeCmd.Stdin = strings.NewReader(caCertBase64)
	caCert, err := decodeCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to decode CA cert: %w", err)
	}

	caCertFile, err := os.CreateTemp(d.tempDir, "roxie-central-ca-*.pem")
	if err != nil {
		return fmt.Errorf("failed to create temp file for CA cert: %w", err)
	}

	d.roxCACertFile = caCertFile.Name()
	if _, err := caCertFile.Write(caCert); err != nil {
		_ = caCertFile.Close()
		_ = os.Remove(d.roxCACertFile)
		return fmt.Errorf("failed to write CA cert: %w", err)
	}
	if err := caCertFile.Close(); err != nil {
		return fmt.Errorf("failed to close CA cert file: %w", err)
	}

	d.logger.Successf("✓ CA certificate saved to: %s", d.roxCACertFile)
	return nil
}

// configureCentralEndpoint configures the central endpoint in the Deployer based on exposure settings.
func (d *Deployer) configureCentralEndpoint(ctx context.Context) error {
	exposure := d.config.Central.GetExposure()
	if d.config.Central.PortForwardingEnabled() {
		// Start port-forward for CLI tool access via localhost:8443
		serviceName := "central"
		if exposure == types.ExposureLoadBalancer {
			_, err := d.waitForLoadBalancer(ctx, d.config.Central.Namespace, "central-loadbalancer", 300)
			if err != nil {
				d.logger.Warningf("LoadBalancer not ready: %v", err)
			} else {
				serviceName = "central-loadbalancer"
			}
		}

		if d.envrcFile != "" {
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
		endpoint, err := d.waitForLoadBalancer(ctx, d.config.Central.Namespace, "central-loadbalancer", 300)
		if err != nil {
			return fmt.Errorf("failed to get LoadBalancer endpoint: %w", err)
		}
		d.centralEndpoint = endpoint
	} else {
		d.centralEndpoint = "central." + d.config.Central.Namespace + ".svc:443"
	}

	if err := d.fetchCentralCACert(ctx); err != nil {
		d.logger.Warningf("Could not fetch CA cert: %v", err)
	}

	if env.RunningInteractively {
		d.logger.Successf("✓ Central is ready at: %s", d.centralEndpoint)
		d.logger.Successf("✓ Admin password: %s", d.centralPassword)
	}

	return nil
}

// deploySecuredClusterOperator deploys SecuredCluster using the operator.
func (d *Deployer) deploySecuredClusterOperator(ctx context.Context) error {
	d.logger.Info("🚀 Deploying SecuredCluster via Operator...")

	needPullSecrets := env.GetCurrentClusterType() != types.ClusterTypeInfraOpenShift4
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
	d.logger.Infof("Using cluster name: %s", clusterName)

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

	if err := d.waitForSecuredClusterReady(ctx); err != nil {
		return fmt.Errorf("failed waiting for SecuredCluster: %w", err)
	}

	if d.config.SecuredCluster.PauseReconciliation {
		d.logger.Infof("Adding pause-reconcile annotation to SecuredCluster")
		err := d.addPauseReconcileAnnotation(ctx, "SecuredCluster", securedClusterCrName, d.config.SecuredCluster.Namespace)
		if err != nil {
			return err
		}
	}

	d.logger.Successf("✓ SecuredCluster '%s' is ready", clusterName)
	return nil
}

func getSecuredClusterResourcesOperator(resourceProfile types.ResourceProfile) map[string]interface{} {
	switch resourceProfile {
	case types.ResourceProfileSmall:
		return map[string]interface{}{
			"spec": map[string]interface{}{
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
	d.logger.Info("Applying SecuredCluster custom resource")

	yamlData, err := yaml.Marshal(cr)
	if err != nil {
		return fmt.Errorf("failed to marshal SecuredCluster CR: %w", err)
	}

	if d.verbose {
		if env.RunningInteractively {
			d.logger.Dim("SecuredCluster CR YAML:")
			d.logger.Dim(string(yamlData))
		} else {
			d.logger.Dim("Skipping emitting SecuredCluster CR in non-interactive mode, because it could leak confidential information")
		}
	}

	result, err := d.runKubectl(ctx, k8s.KubectlOptions{
		Args:  []string{"apply", "-n", d.config.SecuredCluster.Namespace, "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		d.logger.Errorf("kubectl error: %s", result.Stderr)
		return fmt.Errorf("failed to apply SecuredCluster CR: %w", err)
	}

	d.logger.Success("✓ SecuredCluster CR applied")
	return nil
}

// waitForSecuredClusterReady waits for SecuredCluster to be ready
func (d *Deployer) waitForSecuredClusterReady(ctx context.Context) error {
	timeout := d.config.SecuredCluster.DeployTimeout
	d.logger.Infof("⏳ Waiting for SecuredCluster to become ready (timeout: %s)...", timeout)

	// Track seen deployments and their states to avoid duplicate messages
	seenDeployments := make(map[string]string)
	seenPods := make(map[string]string)

	start := time.Now()
	checkInterval := 3 * time.Second

	for time.Since(start) < timeout {
		d.checkDeploymentProgressInNamespace(ctx, d.config.SecuredCluster.Namespace, seenDeployments)

		if d.config.SecuredCluster.EarlyReadiness || d.verbose {
			d.checkPodProgressInNamespace(ctx, d.config.SecuredCluster.Namespace, seenPods)
		}

		allReady := true

		// Check sensor deployment
		result, err := d.runKubectl(ctx, k8s.KubectlOptions{
			Args: []string{"get", "deployment", "sensor", "-n", d.config.SecuredCluster.Namespace, "-o", "jsonpath={.status.readyReplicas}"},
		})
		if err != nil || result.Stdout == "" {
			allReady = false
		} else {
			replicas := strings.TrimSpace(result.Stdout)
			if replicas == "0" || replicas == "" {
				allReady = false
			}
		}

		// Only check additional workloads if early-readiness is not enabled
		if !d.config.SecuredCluster.EarlyReadiness {
			// Check admission-control deployment
			result, err = d.runKubectl(ctx, k8s.KubectlOptions{
				Args: []string{"get", "deployment", "admission-control", "-n", d.config.SecuredCluster.Namespace, "-o", "jsonpath={.status.readyReplicas}"},
			})
			if err != nil || result.Stdout == "" {
				allReady = false
			} else {
				replicas := strings.TrimSpace(result.Stdout)
				if replicas == "0" || replicas == "" {
					allReady = false
				}
			}

			// collector seems to be crashing on some local cluster types/versions.
			// TODO(ROX-34499): skip the check only on local clusters, then?
		}

		if allReady {
			d.logger.Success("✓ SecuredCluster is ready")
			return nil
		}

		time.Sleep(checkInterval)
	}

	return errors.New("timeout waiting for SecuredCluster to become ready")
}
