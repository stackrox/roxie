package deployer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/stackrox/roxie-golang/pkg/helpers"
)

// deployCentralHelm deploys Central using Helm charts
func (d *Deployer) deployCentralHelm(ctx context.Context, resources, exposure string) error {
	d.logger.Info("🚀 Deploying Central via Helm...")

	chartDir, err := os.MkdirTemp("", "central-services-chart-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer d.cleanupTempDir(chartDir, "central chart directory")

	d.logger.PrintWithTimestamp(fmt.Sprintf("Chart directory: %s", chartDir))

	if err := d.generateCentralChart(ctx, chartDir); err != nil {
		return fmt.Errorf("failed to generate central chart: %w", err)
	}

	helmValues, err := d.createCentralValues(resources, exposure)
	if err != nil {
		return fmt.Errorf("failed to create Helm values: %w", err)
	}

	helmValuesYamlBytes, err := yaml.Marshal(helmValues)
	if err != nil {
		return fmt.Errorf("failed to marshal Helm values: %w", err)
	}
	helmValuesYaml := string(helmValuesYamlBytes)

	valuesFile, err := os.CreateTemp("", "central-values-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create values file: %w", err)
	}
	defer os.Remove(valuesFile.Name())
	defer valuesFile.Close()

	if _, err := valuesFile.WriteString(helmValuesYaml); err != nil {
		return fmt.Errorf("failed to write values file: %w", err)
	}
	valuesFile.Close()

	if d.verbose {
		d.logger.Dim("Central values YAML:")
		d.logger.Dim(helmValuesYaml)
	}

	if err := d.verifyHelmChartImages(ctx, chartDir, valuesFile.Name()); err != nil {
		return fmt.Errorf("image verification failed: %w", err)
	}

	// Delete CRDs if they exist (Helm will recreate them)
	d.deleteCRDs(ctx)

	if err := d.ensureNamespaceExists(d.centralNamespace); err != nil {
		return err
	}

	if err := d.prepareNamespace(ctx, d.centralNamespace); err != nil {
		return fmt.Errorf("failed to prepare namespace: %w", err)
	}

	if err := d.installCentralHelmChart(ctx, chartDir, valuesFile.Name()); err != nil {
		return fmt.Errorf("failed to install helm chart: %w", err)
	}

	if err := d.waitForCentralReady(ctx, 600); err != nil {
		return fmt.Errorf("failed waiting for Central: %w", err)
	}

	return d.configureCentralEndpoint(ctx, exposure)
}

// deploySecuredClusterHelm deploys SecuredCluster using Helm charts
func (d *Deployer) deploySecuredClusterHelm(ctx context.Context, resources string) error {
	d.logger.Info("🚀 Deploying SecuredCluster via Helm...")

	clusterName := generateClusterName()

	chartDir, err := os.MkdirTemp("", "secured-cluster-services-chart-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer d.cleanupTempDir(chartDir, "secured-cluster chart directory")

	d.logger.PrintWithTimestamp(fmt.Sprintf("Chart directory: %s", chartDir))

	crsContent, err := d.generateCRS(ctx, clusterName)
	if err != nil {
		return fmt.Errorf("failed to generate CRS: %w", err)
	}

	if err := d.generateSecuredClusterChart(ctx, chartDir); err != nil {
		return fmt.Errorf("failed to generate secured cluster chart: %w", err)
	}

	helmValues, err := d.createSecuredClusterValues(clusterName, resources)
	if err != nil {
		return fmt.Errorf("failed to create values YAML: %w", err)
	}

	helmValuesYamlBytes, err := yaml.Marshal(helmValues)
	if err != nil {
		return fmt.Errorf("failed to marshal Helm values: %w", err)
	}
	helmValuesYaml := string(helmValuesYamlBytes)

	valuesFile, err := os.CreateTemp("", "secured-cluster-values-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create values file: %w", err)
	}
	defer os.Remove(valuesFile.Name())
	defer valuesFile.Close()

	if _, err := valuesFile.WriteString(helmValuesYaml); err != nil {
		return fmt.Errorf("failed to write values file: %w", err)
	}
	valuesFile.Close()

	crsFile, err := os.CreateTemp("", "crs-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create CRS file: %w", err)
	}
	defer os.Remove(crsFile.Name())
	defer crsFile.Close()

	if _, err := crsFile.WriteString(crsContent); err != nil {
		return fmt.Errorf("failed to write CRS file: %w", err)
	}
	crsFile.Close()

	if d.verbose {
		d.logger.Dim("SecuredCluster values YAML:")
		d.logger.Dim(helmValuesYaml)
	}

	if err := d.ensureNamespaceExists(d.sensorNamespace); err != nil {
		return err
	}

	if err := d.prepareNamespace(ctx, d.sensorNamespace); err != nil {
		return fmt.Errorf("failed to prepare namespace: %w", err)
	}

	if err := d.installSecuredClusterHelmChart(ctx, chartDir, valuesFile.Name(), crsFile.Name()); err != nil {
		return fmt.Errorf("failed to install helm chart: %w", err)
	}

	if err := d.waitForSecuredClusterReady(ctx, 600); err != nil {
		return fmt.Errorf("failed waiting for SecuredCluster: %w", err)
	}

	d.logger.Success(fmt.Sprintf("✓ SecuredCluster '%s' is ready", clusterName))
	return nil
}

// generateCentralChart generates the central-services Helm chart using roxctl
func (d *Deployer) generateCentralChart(ctx context.Context, outputDir string) error {
	d.logger.PrintWithTimestamp("Generating central-services chart with roxctl...")

	_, err := d.runRoxctl(ctx, RoxctlOptions{
		Args: []string{
			"helm", "output", "central-services",
			"--remove", "--debug", "--output-dir", outputDir,
		},
		UseAuthentication: false,
	})

	if err != nil {
		return fmt.Errorf("failed to generate central chart: %w", err)
	}

	d.logger.Success("✓ Central chart generated")
	return nil
}

// generateSecuredClusterChart generates the secured-cluster-services Helm chart using roxctl
func (d *Deployer) generateSecuredClusterChart(ctx context.Context, outputDir string) error {
	d.logger.PrintWithTimestamp("Generating secured-cluster-services chart with roxctl...")

	_, err := d.runRoxctl(ctx, RoxctlOptions{
		Args: []string{
			"helm", "output", "secured-cluster-services",
			"--remove", "--debug", "--output-dir", outputDir,
		},
		UseAuthentication: false,
	})

	if err != nil {
		return fmt.Errorf("failed to generate secured cluster chart: %w", err)
	}

	d.logger.Success("✓ SecuredCluster chart generated")
	return nil
}

// createCentralValuesYAML creates the Helm values YAML for Central deployment
func (d *Deployer) createCentralValues(resourcesName, exposure string) (map[string]interface{}, error) {
	base := map[string]interface{}{
		"central": map[string]interface{}{
			"adminPassword": map[string]interface{}{
				"value": d.centralPassword,
			},
			"exposure": d.getCentralExposureConfigHelm(exposure),
			"telemetry": map[string]interface{}{
				"enabled": false,
			},
		},
		"allowNonstandardNamespace": true,
	}

	imageSettings := map[string]interface{}{}
	if d.mainImageTag != "" {
		imageSettings = map[string]interface{}{
			"central": map[string]interface{}{
				"db": map[string]interface{}{
					"image": map[string]interface{}{
						"tag": d.mainImageTag,
					},
				},
				"image": map[string]interface{}{
					"tag": d.mainImageTag,
				},
			},
			"scannerV4": map[string]interface{}{
				"image": map[string]interface{}{
					"tag": d.mainImageTag,
				},
				"db": map[string]interface{}{
					"image": map[string]interface{}{
						"tag": d.mainImageTag,
					},
				},
			},
		}
	}

	resourcesOverlay := d.getCentralResourcesHelm(resourcesName)

	overrides, err := GetOverrides(d.overrideFile, d.overrideSetExpressions)
	if err != nil {
		return nil, fmt.Errorf("failed construct Central Helm values overrides: %w", err)
	}

	merged := helpers.MergeMaps(base, imageSettings, resourcesOverlay, overrides)

	return merged, nil
}

// createSecuredClusterValuesYAML creates the Helm values YAML for SecuredCluster deployment
func (d *Deployer) createSecuredClusterValues(clusterName, resources string) (map[string]interface{}, error) {
	base := map[string]interface{}{
		"clusterName":               clusterName,
		"centralEndpoint":           "https://central.acs-central.svc:443",
		"allowNonstandardNamespace": true,
	}

	imageSettings := map[string]interface{}{}
	if d.mainImageTag != "" {
		imageSettings = map[string]interface{}{
			"image": map[string]interface{}{
				"main": map[string]interface{}{
					"tag": d.mainImageTag,
				},
			},
			"scannerV4":   map[string]interface{}{"tag": d.mainImageTag},
			"scannerV4DB": map[string]interface{}{"tag": d.mainImageTag},
		}
	}

	resourcesOverlay := d.getSecuredClusterResourcesHelm(resources)

	overrides, err := GetOverrides(d.overrideFile, d.overrideSetExpressions)
	if err != nil {
		return nil, fmt.Errorf("failed construct Central Helm values overrides: %w", err)
	}

	merged := helpers.MergeMaps(base, imageSettings, resourcesOverlay, overrides)

	return merged, nil
}

// getCentralExposureConfigHelm returns the exposure configuration for Helm
func (d *Deployer) getCentralExposureConfigHelm(exposure string) map[string]interface{} {
	switch exposure {
	case "loadbalancer":
		return map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"enabled": true,
			},
		}
	case "none":
		return map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"enabled": false,
			},
		}
	default:
		return map[string]interface{}{
			"loadBalancer": map[string]interface{}{
				"enabled": true,
			},
		}
	}
}

// getCentralResourcesHelm returns resource overlays for Central Helm deployment
func (d *Deployer) getCentralResourcesHelm(resourcesName string) map[string]interface{} {
	resourcesSmall := map[string]interface{}{
		"central": map[string]interface{}{
			"resources": centralResourcesSmall,
			"db": map[string]interface{}{
				"resources": centralDbResourcesSmall,
			},
		},
		"scanner": map[string]interface{}{
			"resources": centralScannerResourcesSmall,
			"dbResources": map[string]interface{}{
				"resources": centralScannerDbResourcesSmall,
			},
		},
		"scannerV4": map[string]interface{}{
			"indexer": map[string]interface{}{
				"resources": centralScannerV4IndexerResourcesSmall,
			},
			"matcher": map[string]interface{}{
				"resources": centralScannerV4MatcherResourcesSmall,
			},
			"db": map[string]interface{}{
				"resources": centralScannerV4DbResourcesSmall,
			},
		},
	}
	var resources map[string]interface{}

	if resourcesName == "small" {
		resources = resourcesSmall
	}

	return resources
}

// getSecuredClusterResourcesHelm returns resource overlays for SecuredCluster Helm deployment
func (d *Deployer) getSecuredClusterResourcesHelm(resourcesName string) map[string]interface{} {
	resourcesSmall := map[string]interface{}{
		"sensor": map[string]interface{}{
			"resources": securedClusterSensorResourcesSmall,
		},
		"scanner": map[string]interface{}{
			"disable": true,
		},
		"scannerV4": map[string]interface{}{
			"disable": true,
		},
	}
	var resources map[string]interface{}

	if resourcesName == "small" {
		resources = resourcesSmall
	}

	return resources
}

// verifyHelmChartImages renders the Helm template and verifies that images are pullable
func (d *Deployer) verifyHelmChartImages(ctx context.Context, chartDir, valuesFile string) error {
	d.logger.PrintWithTimestamp("Rendering Helm chart to verify images...")

	cmd := exec.CommandContext(ctx, "helm", "template",
		"-n", d.centralNamespace,
		"stackrox-central-services",
		chartDir,
		"-f", valuesFile)

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to render helm template: %w", err)
	}

	imageRefs := extractImageReferences(string(output))

	if len(imageRefs) == 0 {
		d.logger.Warning("No images found in rendered template")
		return nil
	}

	d.logger.PrintWithTimestamp(fmt.Sprintf("Found %d unique image(s) to verify", len(imageRefs)))
	for _, img := range imageRefs {
		d.logger.Dim(fmt.Sprintf("  - %s", img))
	}

	if !d.imageCache.VerifyImagesPullable(imageRefs...) {
		return errors.New("one or more images not found or not pullable")
	}

	d.logger.Success("✓ All images verified")
	return nil
}

// extractImageReferences extracts unique image references from rendered YAML
func extractImageReferences(renderedYAML string) []string {
	seen := make(map[string]bool)
	var images []string

	lines := strings.Split(renderedYAML, "\n")
	for _, line := range lines {
		// Look for lines like: image: "quay.io/stackrox-io/main:4.8.2"
		if strings.Contains(line, `image: "`) {
			parts := strings.SplitN(line, `image: "`, 2)
			if len(parts) == 2 {
				imageParts := strings.SplitN(parts[1], `"`, 2)
				if len(imageParts) > 0 {
					imageRef := imageParts[0]
					// Only include images with "/main:" tag
					if strings.Contains(imageRef, "/main:") && !seen[imageRef] {
						seen[imageRef] = true
						images = append(images, imageRef)
					}
				}
			}
		}
	}

	return images
}

// installCentralHelmChart installs the central-services Helm chart
func (d *Deployer) installCentralHelmChart(ctx context.Context, chartDir, valuesFile string) error {
	d.logger.PrintWithTimestamp("Installing central-services Helm chart...")

	cmd := exec.CommandContext(ctx, "helm", "install",
		"-n", d.centralNamespace,
		"stackrox-central-services",
		chartDir,
		"-f", valuesFile)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		d.logger.Error(fmt.Sprintf("helm stdout: %s", stdout.String()))
		d.logger.Error(fmt.Sprintf("helm stderr: %s", stderr.String()))
		return fmt.Errorf("failed to install helm chart: %w", err)
	}

	d.logger.Success("✓ Helm chart installed")
	return nil
}

// installSecuredClusterHelmChart installs the secured-cluster-services Helm chart
func (d *Deployer) installSecuredClusterHelmChart(ctx context.Context, chartDir, valuesFile, crsFile string) error {
	d.logger.PrintWithTimestamp("Installing secured-cluster-services Helm chart...")

	cmd := exec.CommandContext(ctx, "helm", "install",
		"-n", d.sensorNamespace,
		"stackrox-secured-cluster-services",
		chartDir,
		"--set-file", fmt.Sprintf("crs.file=%s", crsFile),
		"-f", valuesFile)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		d.logger.Error(fmt.Sprintf("helm stdout: %s", stdout.String()))
		d.logger.Error(fmt.Sprintf("helm stderr: %s", stderr.String()))
		return fmt.Errorf("failed to install helm chart: %w", err)
	}

	d.logger.Success("✓ Helm chart installed")
	return nil
}

// deleteCRDs deletes ACS CRDs (used before Helm deployment)
func (d *Deployer) deleteCRDs(ctx context.Context) {
	crds := []string{
		"centrals.platform.stackrox.io",
		"securedclusters.platform.stackrox.io",
		"securitypolicies.config.stackrox.io",
	}

	d.logger.PrintWithTimestamp("Deleting CRDs...")

	args := append([]string{"delete", "crd", "--ignore-not-found=true"}, crds...)
	d.runKubectl(ctx, KubectlOptions{
		Args:         args,
		IgnoreErrors: true,
	})
}

// SetOverrideFile sets the path to the override values file
func (d *Deployer) SetOverrideFile(path string) {
	d.overrideFile = path
}
