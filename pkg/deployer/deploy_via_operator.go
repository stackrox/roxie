package deployer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/stackrox/roxie-golang/pkg/helpers"
	"gopkg.in/yaml.v3"
)

// deployCentralOperator deploys Central using the operator
func (d *Deployer) deployCentralOperator(ctx context.Context, resources, exposure string) error {
	d.logger.Info("🚀 Deploying Central via Operator...")

	if err := d.ensureCRDsInstalled(ctx); err != nil {
		return fmt.Errorf("failed to ensure CRDs installed: %w", err)
	}

	operatorDeployed := d.isOperatorDeployed(ctx)
	needsDeployment := !operatorDeployed

	if operatorDeployed {
		// Operator exists, check if version is correct
		if d.isOperatorVersionCorrect(ctx) {
			d.logger.Info("✓ Operator already deployed with correct version")
		} else {
			d.logger.Info("🔄 Operator version mismatch, redeploying...")
			needsDeployment = true
		}
	}

	if needsDeployment {
		if err := d.deployOperator(ctx); err != nil {
			return fmt.Errorf("failed to deploy operator: %w", err)
		}
	}

	if err := d.prepareNamespace(ctx, d.centralNamespace); err != nil {
		return fmt.Errorf("failed to prepare namespace: %w", err)
	}

	if err := d.createAdminPasswordSecret(ctx); err != nil {
		return fmt.Errorf("failed to create admin password secret: %w", err)
	}

	centralCR, err := d.createCentralCR(resources, exposure)
	if err != nil {
		return fmt.Errorf("failed to create Central CR: %w", err)
	}

	if err := d.applyCentralCR(ctx, centralCR); err != nil {
		return fmt.Errorf("failed to apply Central CR: %w", err)
	}

	if err := d.waitForCentralReady(ctx, 600); err != nil {
		return fmt.Errorf("failed waiting for Central: %w", err)
	}

	return d.configureCentralEndpoint(ctx, exposure)
}

// isOperatorDeployed checks if the operator is already deployed
func (d *Deployer) isOperatorDeployed(ctx context.Context) bool {
	_, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{"get", "deployment", operatorDeploymentName, "-n", operatorNamespace},
	})
	return err == nil
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

	if currentTag != d.operatorTag {
		d.logger.Info("Operator version mismatch detected:")
		d.logger.Infof("  Current: %s", currentTag)
		d.logger.Infof("  Desired: %s", d.operatorTag)
		return false
	}
	return true
}

// getDeployedOperatorImage gets the image of the currently deployed operator
func (d *Deployer) getDeployedOperatorImage(ctx context.Context) (string, error) {
	result, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{"get", "deployment", operatorDeploymentName, "-n", operatorNamespace,
			"-o", "jsonpath={.spec.template.spec.containers[0].image}"},
	})
	if err != nil {
		return "", err
	}

	image := strings.TrimSpace(result.Stdout)
	return image, nil
}

// prepareNamespace creates pull secrets in the namespace
func (d *Deployer) prepareNamespace(ctx context.Context, namespace string) error {
	d.logger.PrintWithTimestamp(fmt.Sprintf("Preparing namespace %s", namespace))

	if err := d.ensureNamespaceExists(namespace); err != nil {
		return err
	}

	pullSecretYAML, err := d.dockerAuth.CreatePullSecretYAML(namespace)
	if err != nil {
		return fmt.Errorf("could not create pull secret: %w", err)
	}

	_, err = d.runKubectl(ctx, KubectlOptions{
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
			"namespace": d.centralNamespace,
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

	_, err = d.runKubectl(ctx, KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to create admin password secret: %w", err)
	}

	d.logger.Success("✓ Admin password secret created")
	return nil
}

// createCentralCR creates the Central custom resource
func (d *Deployer) createCentralCR(resources, exposure string) (map[string]interface{}, error) {
	base := map[string]interface{}{
		"apiVersion": "platform.stackrox.io/v1alpha1",
		"kind":       "Central",
		"metadata": map[string]interface{}{
			"name":      "stackrox-central-services",
			"namespace": d.centralNamespace,
			"labels": map[string]string{
				"app": "stackrox-central",
			},
		},
		"spec": map[string]interface{}{
			"central": map[string]interface{}{
				"exposure": d.getCentralExposureConfig(exposure),
				"adminPasswordSecret": map[string]interface{}{
					"name": adminPasswordSecretName,
				},
				"telemetry": map[string]interface{}{
					"enabled": false,
				},
			},
			"scanner": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"scaling": map[string]interface{}{
						"autoScaling": "Disabled",
						"replicas":    1,
					},
				},
			},
			"scannerV4": map[string]interface{}{
				"indexer": map[string]interface{}{
					"scaling": map[string]interface{}{
						"autoScaling": "Disabled",
						"replicas":    1,
					},
				},
				"matcher": map[string]interface{}{
					"scaling": map[string]interface{}{
						"autoScaling": "Disabled",
						"replicas":    1,
					},
				},
			},
		},
	}

	resourcesOverlay := d.getCentralResourcesOperator(resources)

	overrides, err := GetOverrides(d.overrideFile, d.overrideSetExpressions)
	if err != nil {
		return nil, fmt.Errorf("failed construct Central CR overrides: %w", err)
	}

	merged := helpers.MergeMaps(base, resourcesOverlay, overrides)

	return merged, nil
}

func (d *Deployer) getCentralResourcesOperator(resourcesName string) map[string]interface{} {
	resourcesSmall := map[string]interface{}{
		"spec": map[string]interface{}{
			"central": map[string]interface{}{
				"resources": centralResourcesSmall,
				"db": map[string]interface{}{
					"resources": centralDbResourcesSmall,
				},
			},
			"scanner": map[string]interface{}{
				"scannerComponent": "Disabled",
			},
			"scannerV4": map[string]interface{}{
				"db": map[string]interface{}{
					"resources": centralScannerV4DbResourcesSmall,
				},
				"indexer": map[string]interface{}{
					"resources": centralScannerV4IndexerResourcesSmall,
				},
				"matcher": map[string]interface{}{
					"resources": centralScannerV4MatcherResourcesSmall,
				},
			},
		},
	}

	var resources map[string]interface{}

	if resourcesName == "small" {
		resources = resourcesSmall
	}

	return resources
}

// getCentralExposureConfig returns the exposure configuration
func (d *Deployer) getCentralExposureConfig(exposure string) map[string]interface{} {
	switch exposure {
	case "loadbalancer":
		return map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"enabled": true,
				"port":    443,
			},
		}
	case "none":
		return map[string]interface{}{
			"nodePort": map[string]interface{}{
				"enabled": false,
			},
			"loadBalancer": map[string]interface{}{
				"enabled": false,
			},
			"route": map[string]interface{}{
				"enabled": false,
			},
		}
	default:
		return map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"enabled": true,
				"port":    443,
			},
		}
	}
}

// applyCentralCR applies the Central CR to the cluster
func (d *Deployer) applyCentralCR(ctx context.Context, cr map[string]interface{}) error {
	d.logger.PrintWithTimestamp("Applying Central custom resource")

	yamlData, err := yaml.Marshal(cr)
	if err != nil {
		return fmt.Errorf("failed to marshal Central CR: %w", err)
	}

	if d.verbose {
		d.logger.Dim("Central CR YAML:")
		d.logger.Dim(string(yamlData))
	}

	result, err := d.runKubectl(ctx, KubectlOptions{
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
func (d *Deployer) waitForCentralReady(ctx context.Context, timeout int) error {
	d.logger.PrintWithTimestamp("⏳ Waiting for Central to become ready...")

	// Track seen deployments and their states to avoid duplicate messages
	seenDeployments := make(map[string]string)
	seenPods := make(map[string]string)

	start := time.Now()
	checkInterval := 3 * time.Second

	for time.Since(start) < time.Duration(timeout)*time.Second {
		// Check for new deployments
		d.checkDeploymentProgress(ctx, seenDeployments)

		// Check for pod events if in early readiness mode or verbose
		if d.earlyReadiness || d.verbose {
			d.checkPodProgress(ctx, seenPods)
		}

		// Check if central deployment is ready
		result, err := d.runKubectl(ctx, KubectlOptions{
			Args: []string{"get", "deployment", "central", "-n", d.centralNamespace, "-o", "jsonpath={.status.readyReplicas}"},
		})
		if err == nil && result.Stdout != "" {
			replicas := strings.TrimSpace(result.Stdout)
			if replicas != "0" && replicas != "" {
				d.logger.Success(fmt.Sprintf("✓ Central is ready (%s replicas)", replicas))
				return nil
			}
		}

		time.Sleep(checkInterval)
	}

	return errors.New("timeout waiting for Central to become ready")
}

// checkDeploymentProgress checks for deployment state changes and reports them
func (d *Deployer) checkDeploymentProgress(ctx context.Context, seenDeployments map[string]string) {
	d.checkDeploymentProgressInNamespace(ctx, d.centralNamespace, seenDeployments)
}

// checkPodProgress checks for pod state changes and reports them
func (d *Deployer) checkPodProgress(ctx context.Context, seenPods map[string]string) {
	d.checkPodProgressInNamespace(ctx, d.centralNamespace, seenPods)
}

// waitForLoadBalancer waits for a LoadBalancer service to get an external IP
func (d *Deployer) waitForLoadBalancer(ctx context.Context, namespace, serviceName string, timeout int) (string, error) {
	d.logger.PrintWithTimestamp(fmt.Sprintf("⏳ Waiting for LoadBalancer %s to get external IP...", serviceName))

	start := time.Now()
	for time.Since(start) < time.Duration(timeout)*time.Second {
		result, err := d.runKubectl(ctx, KubectlOptions{
			Args: []string{"get", "svc", serviceName, "-n", namespace, "-o", "jsonpath={.status.loadBalancer.ingress[0].ip}"},
		})
		if err == nil && result.Stdout != "" {
			ip := strings.TrimSpace(result.Stdout)
			if ip != "" && ip != "<pending>" {
				d.logger.Success(fmt.Sprintf("✓ LoadBalancer IP: %s", ip))
				return fmt.Sprintf("https://%s:443", ip), nil
			}
		}

		// Also check for hostname (some cloud providers use hostname instead of IP)
		result, err = d.runKubectl(ctx, KubectlOptions{
			Args: []string{"get", "svc", serviceName, "-n", namespace, "-o", "jsonpath={.status.loadBalancer.ingress[0].hostname}"},
		})
		if err == nil && result.Stdout != "" {
			hostname := strings.TrimSpace(result.Stdout)
			if hostname != "" && hostname != "<pending>" {
				d.logger.Success(fmt.Sprintf("✓ LoadBalancer hostname: %s", hostname))
				return fmt.Sprintf("https://%s:443", hostname), nil
			}
		}

		time.Sleep(5 * time.Second)
	}

	return "", errors.New("timeout waiting for LoadBalancer to get external address")
}

// fetchCentralCACert fetches the Central CA certificate
func (d *Deployer) fetchCentralCACert(ctx context.Context) error {
	d.logger.PrintWithTimestamp("Fetching Central CA certificate...")

	result, err := d.runKubectl(ctx, KubectlOptions{
		Args: []string{"get", "secret", "central-tls", "-n", d.centralNamespace, "-o", "jsonpath={.data.ca\\.pem}"},
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

	d.roxCACertFile = "/tmp/roxie-ca-cert.pem"
	writeCmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf("cat > %s", d.roxCACertFile))
	writeCmd.Stdin = bytes.NewReader(caCert)
	if err := writeCmd.Run(); err != nil {
		return fmt.Errorf("failed to write CA cert: %w", err)
	}

	d.logger.Success(fmt.Sprintf("✓ CA certificate saved to: %s", d.roxCACertFile))
	return nil
}

// configureCentralEndpoint configures the central endpoint based on exposure settings
// This is shared logic between operator and Helm deployment paths
func (d *Deployer) configureCentralEndpoint(ctx context.Context, exposure string) error {
	if d.portForwardEnabled {
		// Start port-forward for CLI tool access via localhost:8443
		serviceName := "central"
		if exposure == "loadbalancer" {
			_, err := d.waitForLoadBalancer(ctx, d.centralNamespace, "central-loadbalancer", 300)
			if err != nil {
				d.logger.Warning(fmt.Sprintf("LoadBalancer not ready: %v", err))
			} else {
				serviceName = "central-loadbalancer"
			}
		}

		endpoint, err := d.portForward.Start(d.centralNamespace, serviceName, 443, 8443)
		if err != nil {
			return fmt.Errorf("failed to start port-forward: %w", err)
		}
		d.centralEndpoint = endpoint
	} else if exposure == "loadbalancer" {
		endpoint, err := d.waitForLoadBalancer(ctx, d.centralNamespace, "central-loadbalancer", 300)
		if err != nil {
			return fmt.Errorf("failed to get LoadBalancer endpoint: %w", err)
		}
		// Remove https:// prefix if present (waitForLoadBalancer returns https://ip:443)
		d.centralEndpoint = strings.TrimPrefix(endpoint, "https://")
		d.centralEndpoint = strings.TrimSuffix(d.centralEndpoint, ":443")
		d.centralEndpoint = d.centralEndpoint + ":443"
	} else {
		d.centralEndpoint = "central.acs-central.svc:443"
	}

	if err := d.fetchCentralCACert(ctx); err != nil {
		d.logger.Warning(fmt.Sprintf("Could not fetch CA cert: %v", err))
	}

	d.logger.Success(fmt.Sprintf("✓ Central is ready at: %s", d.centralEndpoint))
	d.logger.Success(fmt.Sprintf("✓ Admin password: %s", d.centralPassword))

	return nil
}

// deploySecuredClusterOperator deploys SecuredCluster using the operator
func (d *Deployer) deploySecuredClusterOperator(ctx context.Context, resources string) error {
	d.logger.Info("🚀 Deploying SecuredCluster via Operator...")

	if err := d.ensureCRDsInstalled(ctx); err != nil {
		return fmt.Errorf("failed to ensure CRDs installed: %w", err)
	}

	operatorDeployed := d.isOperatorDeployed(ctx)
	needsDeployment := !operatorDeployed

	if operatorDeployed {
		// Operator exists, check if version is correct
		if d.isOperatorVersionCorrect(ctx) {
			d.logger.Info("✓ Operator already deployed with correct version")
		} else {
			d.logger.Info("🔄 Operator version mismatch, redeploying...")
			needsDeployment = true
		}
	}

	if needsDeployment {
		if err := d.deployOperator(ctx); err != nil {
			return fmt.Errorf("failed to deploy operator: %w", err)
		}
	}

	if err := d.prepareNamespace(ctx, d.sensorNamespace); err != nil {
		return fmt.Errorf("failed to prepare namespace: %w", err)
	}

	clusterName := generateClusterName()

	crsContent, err := d.generateCRS(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("failed to generate CRS: %w", err)
	}

	if err := d.applyCRS(ctx, crsContent); err != nil {
		return fmt.Errorf("failed to apply CRS: %w", err)
	}

	securedClusterCR, err := d.createSecuredClusterCR(clusterName, resources)
	if err != nil {
		return fmt.Errorf("failed to create SecuredCluster CR: %w", err)
	}

	if err := d.applySecuredClusterCR(ctx, securedClusterCR); err != nil {
		return fmt.Errorf("failed to apply SecuredCluster CR: %w", err)
	}

	if err := d.waitForSecuredClusterReady(ctx, 600); err != nil {
		return fmt.Errorf("failed waiting for SecuredCluster: %w", err)
	}

	d.logger.Success(fmt.Sprintf("✓ SecuredCluster '%s' is ready", clusterName))
	return nil
}

// createSecuredClusterCR creates the SecuredCluster custom resource
func (d *Deployer) createSecuredClusterCR(clusterName, resources string) (map[string]interface{}, error) {
	base := map[string]interface{}{
		"apiVersion": "platform.stackrox.io/v1alpha1",
		"kind":       "SecuredCluster",
		"metadata": map[string]interface{}{
			"name":      "stackrox-secured-cluster-services",
			"namespace": d.sensorNamespace,
			"labels": map[string]string{
				"app": "stackrox-secured-cluster",
			},
		},
		"spec": map[string]interface{}{
			"clusterName":     clusterName,
			"centralEndpoint": internalCentralEndpoint,
			"imagePullSecrets": []map[string]string{
				{"name": "stackrox"},
			},
			"admissionControl": map[string]interface{}{
				"replicas": 1,
			},
			"scanner": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"scaling": map[string]interface{}{
						"autoScaling": "Enabled",
						"replicas":    1,
					},
				},
			},
			"scannerV4": map[string]interface{}{
				"indexer": map[string]interface{}{
					"scaling": map[string]interface{}{
						"autoScaling": "Disabled",
						"replicas":    1,
					},
				},
			},
		},
	}

	resourcesOverlay := d.getSecuredClusterResourcesOperator(resources)

	overrides, err := GetOverrides(d.overrideFile, d.overrideSetExpressions)
	if err != nil {
		return nil, fmt.Errorf("failed construct Central CR overrides: %w", err)
	}

	merged := helpers.MergeMaps(base, resourcesOverlay, overrides)

	return merged, nil
}

func (d *Deployer) getSecuredClusterResourcesOperator(resourcesName string) map[string]interface{} {
	resourcesSmall := map[string]interface{}{
		"spec": map[string]interface{}{
			"sensor": map[string]interface{}{
				"resources": securedClusterSensorResourcesSmall,
			},
			"scanner": map[string]interface{}{
				"scannerComponent": "Disabled",
			},
			"scannerV4": map[string]interface{}{
				"scannerComponent": "Disabled",
			},
		},
	}
	var resources map[string]interface{}

	if resourcesName == "small" {
		resources = resourcesSmall
	}

	return resources
}

// applySecuredClusterCR applies the SecuredCluster CR to the cluster
func (d *Deployer) applySecuredClusterCR(ctx context.Context, cr map[string]interface{}) error {
	d.logger.PrintWithTimestamp("Applying SecuredCluster custom resource")

	yamlData, err := yaml.Marshal(cr)
	if err != nil {
		return fmt.Errorf("failed to marshal SecuredCluster CR: %w", err)
	}

	if d.verbose {
		d.logger.Dim("SecuredCluster CR YAML:")
		d.logger.Dim(string(yamlData))
	}

	result, err := d.runKubectl(ctx, KubectlOptions{
		Args:  []string{"apply", "-n", d.sensorNamespace, "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		d.logger.Error(fmt.Sprintf("kubectl error: %s", result.Stderr))
		return fmt.Errorf("failed to apply SecuredCluster CR: %w", err)
	}

	d.logger.Success("✓ SecuredCluster CR applied")
	return nil
}

// waitForSecuredClusterReady waits for SecuredCluster to be ready
func (d *Deployer) waitForSecuredClusterReady(ctx context.Context, timeout int) error {
	d.logger.PrintWithTimestamp("⏳ Waiting for SecuredCluster to become ready...")

	// Track seen deployments and their states to avoid duplicate messages
	seenDeployments := make(map[string]string)
	seenPods := make(map[string]string)

	start := time.Now()
	checkInterval := 3 * time.Second

	for time.Since(start) < time.Duration(timeout)*time.Second {
		// Check for new deployments
		d.checkDeploymentProgressInNamespace(ctx, d.sensorNamespace, seenDeployments)

		// Check for pod events if in early readiness mode or verbose
		if d.earlyReadiness || d.verbose {
			d.checkPodProgressInNamespace(ctx, d.sensorNamespace, seenPods)
		}

		allReady := true

		// Check sensor deployment
		result, err := d.runKubectl(ctx, KubectlOptions{
			Args: []string{"get", "deployment", "sensor", "-n", d.sensorNamespace, "-o", "jsonpath={.status.readyReplicas}"},
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
		if !d.earlyReadiness {
			// Check admission-control deployment
			result, err = d.runKubectl(ctx, KubectlOptions{
				Args: []string{"get", "deployment", "admission-control", "-n", d.sensorNamespace, "-o", "jsonpath={.status.readyReplicas}"},
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
		}

		if allReady {
			d.logger.Success("✓ SecuredCluster is ready")
			return nil
		}

		time.Sleep(checkInterval)
	}

	return errors.New("timeout waiting for SecuredCluster to become ready")
}
