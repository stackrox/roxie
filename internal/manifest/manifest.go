package manifest

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/env"
	"github.com/stackrox/roxie/internal/k8s"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/types"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	roxieNamespace     = "roxie"
	manifestSecretName = "roxie-manifest"
	manifestKey        = "manifest"
)

// RoxieManifest represents the data stored in the roxie manifest secret.
// We include the whole deployer Config for reproducibility purposes.
type RoxieManifest struct {
	RoxieEnvironment types.RoxieEnvironment `yaml:"roxieEnvironment"`
	Config           deployer.Config        `yaml:"config"`
}

func manifestToSecret(m RoxieManifest) (*unstructured.Unstructured, error) {
	manifestYAML, err := yaml.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      manifestSecretName,
				"namespace": roxieNamespace,
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "roxie",
				},
			},
			"type": "Opaque",
			"stringData": map[string]any{
				manifestKey: string(manifestYAML),
			},
		},
	}

	return secret, nil
}

func CreateManifestSecretOnCluster(ctx context.Context, log *logger.Logger, m RoxieManifest) error {
	secret, err := manifestToSecret(m)
	if err != nil {
		return fmt.Errorf("failed to convert manifest to secret: %w", err)
	}

	yamlData, err := yaml.Marshal(secret.Object)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest secret: %w", err)
	}

	if err := ensureRoxieNamespace(ctx, log); err != nil {
		return fmt.Errorf("failed to ensure roxie namespace exists: %w", err)
	}

	_, err = k8s.RunKubectl(ctx, log, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(yamlData),
	})
	if err != nil {
		return fmt.Errorf("failed to apply manifest secret to cluster: %w", err)
	}

	log.Dim("roxie manifest secret applied")
	return nil
}

func LoadManifestSecret(ctx context.Context, log *logger.Logger) (*RoxieManifest, error) {
	obj, err := k8s.RetrieveResourceFromCluster(ctx, log, roxieNamespace, "secret", manifestSecretName)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve manifest secret: %w", err)
	}

	manifestB64, found, err := unstructured.NestedString(obj.Object, "data", manifestKey)
	if err != nil {
		return nil, fmt.Errorf("failed to extract secret data: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("manifest secret missing key %q", manifestKey)
	}

	manifestBytes, err := base64.StdEncoding.DecodeString(manifestB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode %s: %w", manifestKey, err)
	}

	var m RoxieManifest
	if err := yaml.Unmarshal(manifestBytes, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &m, nil
}

func DeleteManifestSecret(ctx context.Context, log *logger.Logger) error {
	_, err := k8s.RunKubectl(ctx, log, k8s.KubectlOptions{
		Args:         []string{"delete", "secret", manifestSecretName, "-n", roxieNamespace, "--ignore-not-found=true"},
		IgnoreErrors: true,
	})
	return err
}

func DeleteRoxieNamespace(ctx context.Context, log *logger.Logger) error {
	_, err := k8s.RunKubectl(ctx, log, k8s.KubectlOptions{
		Args:         []string{"delete", "namespace", roxieNamespace, "--ignore-not-found=true"},
		IgnoreErrors: true,
	})
	return err
}

func ensureRoxieNamespace(ctx context.Context, log *logger.Logger) error {
	ns := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name": roxieNamespace,
				"labels": map[string]any{
					"app.kubernetes.io/managed-by": "roxie",
				},
			},
		},
	}
	nsYAML, err := yaml.Marshal(ns.Object)
	if err != nil {
		return fmt.Errorf("failed to marshal namespace: %w", err)
	}
	_, err = k8s.RunKubectl(ctx, log, k8s.KubectlOptions{
		Args:  []string{"apply", "-f", "-"},
		Stdin: bytes.NewReader(nsYAML),
	})
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", roxieNamespace, err)
	}

	return nil
}

func ManifestToCentralDeploymentInfo(ctx context.Context, log *logger.Logger, tempDir string, m *RoxieManifest) (types.CentralDeploymentInfo, error) {
	roxieEnv := m.RoxieEnvironment

	caCertFile, err := fetchCACertForShell(ctx, log, m.Config.Central.Namespace, tempDir)
	if err != nil {
		// Nothing we expect to happen, but in any case, don't let the deployment fail here.
		log.Warningf("Could not fetch CA cert: %v", err)
	}

	return types.CentralDeploymentInfo{
		Endpoint:    roxieEnv.RoxEndpoint,
		Username:    roxieEnv.RoxUsername,
		Password:    roxieEnv.RoxAdminPassword,
		KubeContext: env.GetCurrentContext(),
		Exposure:    m.Config.Central.GetExposure(),
		CACertFile:  caCertFile,
	}, nil
}

func fetchCACertForShell(ctx context.Context, log *logger.Logger, centralNamespace, tempDir string) (string, error) {
	log.Info("Fetching Central CA certificate...")

	result, err := k8s.RunKubectl(ctx, log, k8s.KubectlOptions{
		Args: []string{"get", "secret", "central-tls", "-n", centralNamespace, "-o", "jsonpath={.data.ca\\.pem}"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to get CA cert from secret: %w", err)
	}

	caCertBase64 := strings.TrimSpace(result.Stdout)
	if caCertBase64 == "" {
		return "", errors.New("CA certificate is empty")
	}

	caCert, err := base64.StdEncoding.DecodeString(caCertBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode CA cert: %w", err)
	}

	caCertFile, err := os.CreateTemp(tempDir, "roxie-ca-*.pem")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for CA cert: %w", err)
	}

	if _, err := caCertFile.Write(caCert); err != nil {
		_ = caCertFile.Close()
		_ = os.Remove(caCertFile.Name())
		return "", fmt.Errorf("failed to write CA cert: %w", err)
	}
	if err := caCertFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close CA cert file: %w", err)
	}

	log.Successf("✓ CA certificate saved to: %s", caCertFile.Name())
	return caCertFile.Name(), nil
}
