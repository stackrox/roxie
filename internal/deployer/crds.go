package deployer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// requiredCRDs lists the CRDs required for ACS operator.
var requiredCRDs = []string{
	"centrals.platform.stackrox.io",
	"securedclusters.platform.stackrox.io",
	"securitypolicies.config.stackrox.io",
}

// ensureCRDsInstalled ensures required CRDs exist, installing them from the provided bundle directory.
func (d *Deployer) ensureCRDsInstalled(ctx context.Context, bundleDir string) error {
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
		d.logger.Warningf("Missing CRDs detected (%s)", strings.Join(missing, ", "))

		crdFiles, err := d.identifyCRDFileNames(bundleDir)
		if err != nil {
			return err
		}

		return d.applyCRDsToCluster(ctx, crdFiles)
	}

	return nil
}

// identifyCRDFileNames identifies CRD files in the bundle directory.
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

		// TODO(#91): The following detection logic does not seem particularly robust. We should
		// probably parse the YAML and check api group and kind fields.
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

// deleteCRDs deletes RHACS CRDs from the cluster
func (d *Deployer) deleteCRDs(ctx context.Context) {
	d.logger.Info("Deleting CRDs...")

	args := append([]string{"delete", "crd", "--ignore-not-found=true"}, requiredCRDs...)
	d.runKubectl(ctx, KubectlOptions{
		Args:         args,
		IgnoreErrors: true,
	})
}
