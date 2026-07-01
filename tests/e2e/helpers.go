//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	deployTimeout   = 30 * time.Minute
	teardownTimeout = 10 * time.Minute
)

var (
	commonDeployArgs = []string{"--resources=small", "--skip-user-config"}

	roxieBinary = "roxie"
)

func teardownAllDeployments() error {
	fmt.Println("=== Tearing down all deployments before running tests ===")

	ctx, cancel := context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()

	// Teardown standard deployments
	cmd := exec.CommandContext(ctx, roxieBinary, "teardown", "--skip-user-config")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Warning: teardown command failed: %w", err)
	}

	// Teardown single-namespace deployments
	ctx, cancel = context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()
	cmd = exec.CommandContext(ctx, roxieBinary, "teardown", "--skip-user-config", "--single-namespace")
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

var clusterDumpNamespaces = []string{
	"rhacs-operator-system",
	"acs-central",
	"acs-sensor",
	"stackrox",
}

func dumpClusterStateOnFailure(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		dumpClusterResources(t)
	})
}

func dumpClusterResources(t *testing.T) {
	t.Helper()
	fmt.Fprintf(os.Stderr, "=== CLUSTER RESOURCE DUMP (test %s failed) ===\n", t.Name())

	runKubectlDump("get", "namespaces")

	for _, ns := range clusterDumpNamespaces {
		fmt.Fprintf(os.Stderr, "--- Namespace: %s ---\n", ns)
		runKubectlDump("get", "pods", "-n", ns, "-o", "wide")
		runKubectlDump("describe", "pods", "-n", ns)
		runKubectlDump("get", "deployments", "-n", ns, "-o", "wide")
		runKubectlDump("describe", "deployments", "-n", ns)
		runKubectlDump("get", "daemonsets", "-n", ns, "-o", "wide")
		runKubectlDump("describe", "daemonsets", "-n", ns)
		runKubectlDump("get", "events", "-n", ns, "--sort-by=.lastTimestamp")
		dumpLogsForFailingPods(ns)
	}

	dumpACSCustomResources()
	dumpOLMResources()

	fmt.Fprintln(os.Stderr, "=== END CLUSTER RESOURCE DUMP ===")
}

func dumpACSCustomResources() {
	fmt.Fprintln(os.Stderr, "--- ACS Custom Resources ---")
	for _, ns := range clusterDumpNamespaces {
		runKubectlDump("get", "centrals.platform.stackrox.io", "-n", ns, "-o", "yaml")
		runKubectlDump("get", "securedclusters.platform.stackrox.io", "-n", ns, "-o", "yaml")
	}
}

func dumpOLMResources() {
	cmd := exec.Command("kubectl", "api-resources", "--api-group=operators.coreos.com", "-o", "name")
	output, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(output)) == "" {
		fmt.Fprintln(os.Stderr, "[dump] OLM not installed, skipping OLM resource dump")
		return
	}

	fmt.Fprintln(os.Stderr, "--- OLM Resources ---")
	operatorNamespace := "rhacs-operator-system"
	runKubectlDump("get", "subscriptions.operators.coreos.com", "-n", operatorNamespace, "-o", "wide")
	runKubectlDump("describe", "subscriptions.operators.coreos.com", "-n", operatorNamespace)
	runKubectlDump("get", "installplans.operators.coreos.com", "-n", operatorNamespace, "-o", "wide")
	runKubectlDump("describe", "installplans.operators.coreos.com", "-n", operatorNamespace)
	runKubectlDump("get", "catalogsources.operators.coreos.com", "-n", operatorNamespace, "-o", "wide")
	runKubectlDump("describe", "catalogsources.operators.coreos.com", "-n", operatorNamespace)
	runKubectlDump("get", "clusterserviceversions.operators.coreos.com", "-n", operatorNamespace, "-o", "wide")
	runKubectlDump("describe", "clusterserviceversions.operators.coreos.com", "-n", operatorNamespace)
	runKubectlDump("get", "operatorgroups.operators.coreos.com", "-n", operatorNamespace, "-o", "wide")
	runKubectlDump("describe", "operatorgroups.operators.coreos.com", "-n", operatorNamespace)
}

func runKubectlDump(args ...string) {
	fmt.Fprintf(os.Stderr, "## kubectl %s\n", strings.Join(args, " "))
	cmd := exec.Command("kubectl", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "kubectl failed: %v\n", err)
	}
	fmt.Fprintln(os.Stderr)
}

func dumpLogsForFailingPods(namespace string) {
	cmd := exec.Command("kubectl", "get", "pods", "-n", namespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}{\"\\t\"}{.status.phase}{\"\\n\"}{end}")
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[dump] failed to list pods in %s: %v\n", namespace, err)
		return
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		podName, phase := parts[0], parts[1]
		if phase == "Running" || phase == "Succeeded" {
			continue
		}
		fmt.Fprintf(os.Stderr, "[dump] logs for pod %s/%s (phase=%s):\n", namespace, podName, phase)
		runKubectlDump("logs", "-n", namespace, podName, "--all-containers", "--tail=100")
		runKubectlDump("logs", "-n", namespace, podName, "--all-containers", "--previous", "--tail=50")
	}
}

func testCentralAPI(t *testing.T, endpoint, caCertFile string) {
	t.Helper()

	caCert, err := os.ReadFile(caCertFile)
	require.NoError(t, err, "Failed to read CA cert file: %s", caCertFile)
	certPool := x509.NewCertPool()
	require.True(t, certPool.AppendCertsFromPEM(caCert), "Failed to parse CA certificate")

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    certPool,
				ServerName: "central.stackrox.svc",
			},
		},
	}
	resp, err := client.Get(fmt.Sprintf("https://%s/v1/ping", endpoint))
	require.NoError(t, err, "Failed to reach Central via %s", endpoint)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Central API returned unexpected status")
	t.Logf("Central at %s responded with status: %d", endpoint, resp.StatusCode)
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
