package deployer

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// generateCRS generates the Central Resource Secret using roxctl
func (d *Deployer) generateCRS(ctx context.Context, clusterName string) (string, error) {
	crsName := fmt.Sprintf("%s-crs", clusterName)
	d.logger.Infof("Generating CRS named %q with roxctl...", crsName)

	result, err := d.runRoxctl(ctx, RoxctlOptions{
		Args: []string{
			"-e", d.centralEndpoint,
			"central",
			"crs",
			"generate",
			crsName,
			"--output=-", // Output to stdout
		},
		UseAuthentication: true,
		MaxAttempts:       5,
		RetryDelay:        10,
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate CRS: %w", err)
	}

	crsContent := strings.TrimSpace(result.Stdout)
	if crsContent == "" {
		return "", errors.New("CRS content is empty")
	}

	d.logger.Success("✓ CRS generated")
	return crsContent, nil
}

// applyCRS applies the CRS content to the sensor namespace
func (d *Deployer) applyCRS(ctx context.Context, crsContent string) error {
	d.logger.Info("Applying CRS to sensor namespace")

	result, err := d.runKubectl(ctx, KubectlOptions{
		Args:  []string{"apply", "-n", d.sensorNamespace, "-f", "-"},
		Stdin: strings.NewReader(crsContent),
	})
	if err != nil {
		d.logger.Errorf("kubectl stdout: %s", result.Stdout)
		d.logger.Errorf("kubectl stderr: %s", result.Stderr)
		return fmt.Errorf("failed to apply CRS: %w\nStderr: %s", err, result.Stderr)
	}

	d.logger.Success("✓ CRS applied")
	return nil
}
