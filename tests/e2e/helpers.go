//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// TODO(#91): We should come up with some auto-updating of this on ACS releases.
	// Don't think we should directly inject nightlies here.
	defaultMainImageTag = "4.10.1"
	deployTimeout       = 30 * time.Minute
	teardownTimeout     = 10 * time.Minute
)

var (
	commonDeployArgs              = []string{"--port-forwarding", "--exposure=none", "--resources=small"}
	commonDeployArgsNoPortForward = []string{"--exposure=loadbalancer", "--resources=small"}

	roxieBinary = "roxie"
)

// TODO(#91): maybe put the helper functions in a separate file?
func teardownAllDeployments() error {
	fmt.Println("=== Tearing down all deployments before running tests ===")

	ctx, cancel := context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()

	// Teardown standard deployments
	cmd := exec.CommandContext(ctx, roxieBinary, "teardown")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Warning: teardown command failed: %w", err)
	}

	// Teardown single-namespace deployments
	ctx, cancel = context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()
	cmd = exec.CommandContext(ctx, roxieBinary, "teardown", "--single-namespace")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Warning: teardown --single-namespace command failed: %w", err)
	}

	fmt.Println("=== All deployments have been torn down ===")
	return nil
}

func requireBinary(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func requireKubeContext() (string, error) {
	cmd := exec.Command("kubectl", "config", "current-context")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("no kubectl context available: %w", err)
	}

	ctx := strings.TrimSpace(string(output))
	if ctx == "" {
		return "", fmt.Errorf("kubectl context is empty")
	}

	return ctx, nil
}

func runCommand(t *testing.T, timeout time.Duration, env map[string]string, args ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	// Set environment
	if env != nil {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Stream output to both test log and buffers
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &testWriter{t: t, prefix: "[stdout] ", buf: &stdout}
	cmd.Stderr = &testWriter{t: t, prefix: "[stderr] ", buf: &stderr}

	t.Logf("Running: %s", strings.Join(args, " "))

	err := cmd.Run()
	if err != nil {
		t.Logf("Command failed: %v", err)
		t.Fatalf("Command failed: %v", err)
	}
	t.Logf("Command completed successfully")
}

// testWriter writes to both test log and a buffer
type testWriter struct {
	t      *testing.T
	prefix string
	buf    *bytes.Buffer
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	// Write to buffer
	n, err = w.buf.Write(p)
	if err != nil {
		return n, err
	}

	// Also log each line to test output
	lines := bytes.Split(p, []byte("\n"))
	for _, line := range lines {
		if len(line) > 0 {
			w.t.Logf("%s%s", w.prefix, string(line))
		}
	}

	return n, nil
}

func verifyNamespaceExists(t *testing.T, namespace string) {
	t.Helper()

	cmd := exec.Command("kubectl", "get", "namespace", namespace)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Namespace %s does not exist", namespace)
	}
}

func verifyNamespaceHasLabel(t *testing.T, namespace, key, value string) {
	t.Helper()

	cmd := exec.Command("kubectl", "get", "namespace", namespace,
		"-o", "jsonpath={.metadata.labels}")
	output, err := cmd.Output()
	require.NoError(t, err, "Failed to retrieve labels for namespace %s", namespace)

	var labels map[string]string
	err = json.Unmarshal(output, &labels)
	require.NoError(t, err, "JSON unmarshalling of namespace labels failed")

	actualValue := labels[key]
	assert.Equal(t, value, actualValue, "Namespace %s is missing the correct label for %s", namespace, key)
}

func doesDeploymentExist(t *testing.T, namespace string, name string) bool {
	t.Helper()

	cmd := exec.Command("kubectl", "-n", namespace, "get", "deployments", "-o", "name")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get deployments in namespace %s: %v", namespace, err)
	}
	return strings.Contains(string(output), name)
}

func loadEnvrcFile(path string) (map[string]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	env := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "export ") {
			continue
		}

		// Remove "export " prefix
		line = strings.TrimPrefix(line, "export ")

		// Split on first =
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes
		value = strings.Trim(value, "\"")

		env[key] = value
	}

	return env, nil
}

func verifyCentralInstalled(t *testing.T, namespace string) {
	t.Helper()

	if !doesDeploymentExist(t, namespace, "central") {
		t.Fatalf("Central is not installed in namespace %s", namespace)
	}
}

func verifySecuredClusterInstalled(t *testing.T, namespace string) {
	t.Helper()

	if !doesDeploymentExist(t, namespace, "sensor") {
		t.Fatalf("Secured cluster is not installed in namespace %s", namespace)
	}
}

func verifyCentralNotInstalled(t *testing.T, namespace string) {
	t.Helper()

	if doesDeploymentExist(t, namespace, "central") {
		t.Fatalf("Central is installed in namespace %s", namespace)
	}
}

func verifySecuredClusterNotInstalled(t *testing.T, namespace string) {
	t.Helper()

	if doesDeploymentExist(t, namespace, "sensor") {
		t.Fatalf("Secured cluster is installed in namespace %s", namespace)
	}
}

func verifyAnnotation(t *testing.T, resourceType, resourceName, namespace, annotationKey, expectedValue string) {
	t.Helper()

	cmd := exec.Command("kubectl", "get", resourceType, resourceName, "-n", namespace, "-o", "jsonpath={.metadata.annotations}")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Failed to get annotation %s on %s/%s in namespace %s: %v", annotationKey, resourceType, resourceName, namespace, err)
	}

	annotations := make(map[string]string)
	err = json.Unmarshal(output, &annotations)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	currentValue := annotations[annotationKey]
	if currentValue != expectedValue {
		t.Fatalf("Annotation %s on %s/%s has incorrect value. Expected: %s, Got: %s", annotationKey, resourceType, resourceName, expectedValue, currentValue)
	}

	t.Logf("✓ Annotation %s=%s verified on %s/%s", annotationKey, expectedValue, resourceType, resourceName)
}
