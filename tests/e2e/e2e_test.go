//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	defaultMainImageTag = "4.8.2"
	deployTimeout       = 30 * time.Minute
	teardownTimeout     = 10 * time.Minute
)

var (
	commonDeployArgs              = []string{"--port-forwarding", "--exposure=none", "--resources=small"}
	commonDeployArgsNoPortForward = []string{"--exposure=loadbalancer", "--resources=small"}

	roxieBinary string
)

func TestMain(m *testing.M) {
	// Setup: verify prerequisites
	if err := requireBinary("kubectl"); err != nil {
		fmt.Fprintf(os.Stderr, "kubectl not found: %v\n", err)
		os.Exit(1)
	}

	// Set default MAIN_IMAGE_TAG if not set
	if os.Getenv("MAIN_IMAGE_TAG") == "" {
		os.Setenv("MAIN_IMAGE_TAG", defaultMainImageTag)
	}

	// Verify kubectl context
	ctx, err := requireKubeContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "kubectl context check failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Using kubectl context: %s\n", ctx)

	// Find roxie binary
	roxieBinary = findRoxieBinary()
	if roxieBinary == "" {
		fmt.Fprintln(os.Stderr, "roxie binary not found")
		os.Exit(1)
	}
	fmt.Printf("Using roxie binary: %s\n", roxieBinary)

	// Teardown all deployments before running tests
	if err := teardownAllDeployments(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: teardown all deployments failed: %v\n", err)
		os.Exit(1)
	}

	// Run tests
	os.Exit(m.Run())
}

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

func findRoxieBinary() string {
	// Try current directory
	if _, err := os.Stat("./roxie"); err == nil {
		return "./roxie"
	}

	// Try ../.. (from tests/e2e to repo root)
	repoRoot := filepath.Join("..", "..")
	roxiePath := filepath.Join(repoRoot, "roxie")
	if _, err := os.Stat(roxiePath); err == nil {
		return roxiePath
	}

	return ""
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

func TestDeployCentralAndSecuredCluster(t *testing.T) {
	if os.Getenv("SKIP_OPERATOR_TESTS") != "" {
		t.Skip("SKIP_OPERATOR_TESTS is set")
	}

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp("", ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()
	defer os.Remove(envrcPath)

	// Deploy central
	t.Log("=== Deploying central ===")
	args := append([]string{roxieBinary, "deploy", "--early-readiness", "central", "--envrc", envrcPath}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout, nil, args...)

	// Load environment from envrc file for secured-cluster deployment
	envrcEnv, err := loadEnvrcFile(envrcPath)
	if err != nil {
		t.Fatalf("Failed to load envrc file: %v", err)
	}
	t.Log("Loaded environment from envrc file for secured-cluster")

	t.Log("=== Deploying secured-cluster ===")
	args = append([]string{roxieBinary, "deploy", "--early-readiness", "secured-cluster"}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout, envrcEnv, args...)

	// Verify namespaces
	t.Log("Verifying namespace: acs-central")
	verifyNamespaceExists(t, "acs-central")

	t.Log("Verifying namespace: acs-sensor")
	verifyNamespaceExists(t, "acs-sensor")

	// Brief pause before next test
	time.Sleep(5 * time.Second)
}

func TestTeardownCentralAndSecuredCluster(t *testing.T) {
	if os.Getenv("SKIP_OPERATOR_TESTS") != "" {
		t.Skip("SKIP_OPERATOR_TESTS is set")
	}

	t.Log("=== Tearing down central and secured-cluster ===")
	args := []string{roxieBinary, "teardown", "both"}
	runCommand(t, teardownTimeout, nil, args...)

	t.Log("Verifying components are removed")
	verifyCentralNotInstalled(t, "acs-central")
	verifySecuredClusterNotInstalled(t, "acs-sensor")
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

func TestDeployBothComponentsTogether(t *testing.T) {
	if os.Getenv("SKIP_OPERATOR_TESTS") != "" {
		t.Skip("SKIP_OPERATOR_TESTS is set")
	}

	// Create temporary envrc file
	envrcFile, err := os.CreateTemp("", ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()
	defer os.Remove(envrcPath)

	t.Log("=== Deploying both components ===")
	// We also test --pause-reconciliation flag here.
	args := append([]string{roxieBinary, "deploy", "both", "--pause-reconciliation", "--envrc", envrcPath}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout*2, nil, args...)

	t.Log("Verifying namespace: acs-central")
	verifyNamespaceExists(t, "acs-central")

	t.Log("Verifying namespace: acs-sensor")
	verifyNamespaceExists(t, "acs-sensor")

	// Verify Central has the pause-reconcile annotation.
	t.Log("Verifying pause-reconcile annotation on Central CR")
	verifyAnnotation(t, "central", "stackrox-central-services", "acs-central", "stackrox.io/pause-reconcile", "true")

	// Verify SecuredCluster has the pause-reconcile annotation.
	t.Log("Verifying pause-reconcile annotation on SecuredCluster CR")
	verifyAnnotation(t, "securedcluster", "stackrox-secured-cluster-services", "acs-sensor", "stackrox.io/pause-reconcile", "true")

}

func TestDeployBothComponentsTogetherInSingleNamespace(t *testing.T) {
	if os.Getenv("SKIP_OPERATOR_TESTS") != "" {
		t.Skip("SKIP_OPERATOR_TESTS is set")
	}

	// Create temporary envrc file.
	envrcFile, err := os.CreateTemp("", ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()
	defer os.Remove(envrcPath)

	t.Log("=== Deploying both components in single namespace ===")
	args := append([]string{roxieBinary, "deploy", "both", "--single-namespace", "--early-readiness", "--envrc", envrcPath}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout*2, nil, args...)

	verifyCentralInstalled(t, "stackrox")
	verifySecuredClusterInstalled(t, "stackrox")

	t.Log("=== Tearing down both components in single namespace ===")
	args = []string{roxieBinary, "teardown", "--single-namespace"}
	runCommand(t, teardownTimeout, nil, args...)

	verifyCentralNotInstalled(t, "stackrox")
	verifySecuredClusterNotInstalled(t, "stackrox")
}

func TestDeployCentralAndSecuredClusterViaHelm(t *testing.T) {
	// Create temporary envrc file
	envrcFile, err := os.CreateTemp("", ".envrc.roxie-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp envrc: %v", err)
	}
	envrcPath := envrcFile.Name()
	envrcFile.Close()
	defer os.Remove(envrcPath)

	t.Log("=== Deploying central via Helm ===")
	args := append([]string{roxieBinary, "deploy", "--early-readiness", "central", "--helm", "--envrc", envrcPath}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout*2, nil, args...)

	// Load environment from envrc file for secured-cluster deployment
	envrcEnv, err := loadEnvrcFile(envrcPath)
	if err != nil {
		t.Fatalf("Failed to load envrc file: %v", err)
	}
	t.Log("Loaded environment from envrc file for secured-cluster")

	t.Log("=== Deploying secured-cluster via Helm ===")
	args = append([]string{roxieBinary, "deploy", "--early-readiness", "secured-cluster", "--helm"}, commonDeployArgsNoPortForward...)
	runCommand(t, deployTimeout*2, envrcEnv, args...)

	t.Log("Verifying namespace: acs-central")
	verifyNamespaceExists(t, "acs-central")

	t.Log("Verifying namespace: acs-sensor")
	verifyNamespaceExists(t, "acs-sensor")
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
