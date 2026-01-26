package deployer

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestEnvVarToImageName tests the conversion of RELATED_IMAGE_* env vars to image names
func TestEnvVarToImageName(t *testing.T) {
	tests := []struct {
		envVar   string
		expected string
	}{
		{"RELATED_IMAGE_MAIN", "main"},
		{"RELATED_IMAGE_SCANNER", "scanner"},
		{"RELATED_IMAGE_SCANNER_DB", "scanner-db"},
		{"RELATED_IMAGE_SCANNER_V4_DB", "scanner-v4-db"},
		{"RELATED_IMAGE_SCANNER_V4", "scanner-v4"},
		{"RELATED_IMAGE_SCANNER_V4_INDEXER", "scanner-v4-indexer"},
		{"RELATED_IMAGE_SCANNER_V4_MATCHER", "scanner-v4-matcher"},
		{"RELATED_IMAGE_CENTRAL_DB", "central-db"},
		{"RELATED_IMAGE_COLLECTOR", "collector"},
	}

	for _, tt := range tests {
		t.Run(tt.envVar, func(t *testing.T) {
			result := envVarToImageName(tt.envVar)
			if result != tt.expected {
				t.Errorf("envVarToImageName(%q) = %q, want %q", tt.envVar, result, tt.expected)
			}
		})
	}
}

// TestPatchCSVWithLocalImages_ScannerV4Mapping tests that scanner-v4-indexer and scanner-v4-matcher
// both get mapped to the scanner-v4 image
func TestPatchCSVWithLocalImages_ScannerV4Mapping(t *testing.T) {
	csvContent := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: rhacs-operator.v4.0.0
spec:
  install:
    spec:
      deployments:
      - name: rhacs-operator-controller-manager
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/rhacs-eng/stackrox-operator:4.0.0
                env:
                - name: RELATED_IMAGE_SCANNER_V4
                  value: quay.io/rhacs-eng/scanner-v4:4.0.x-nightly
                - name: RELATED_IMAGE_SCANNER_V4_INDEXER
                  value: quay.io/rhacs-eng/scanner-v4:4.0.x-nightly
                - name: RELATED_IMAGE_SCANNER_V4_MATCHER
                  value: quay.io/rhacs-eng/scanner-v4:4.0.x-nightly
`

	csvFile := filepath.Join(t.TempDir(), "test-csv.yaml")
	if err := os.WriteFile(csvFile, []byte(csvContent), 0644); err != nil {
		t.Fatalf("Failed to write test CSV: %v", err)
	}

	// Only have scanner-v4 locally (not separate indexer/matcher)
	localImages := map[string]string{
		"scanner-v4:4.0.0-local": "quay.io/rhacs-eng/scanner-v4:4.0.0-local",
	}

	err := patchCSVWithLocalImages(csvFile, "4.0.0-local", localImages)
	if err != nil {
		t.Fatalf("patchCSVWithLocalImages failed: %v", err)
	}

	// Read and parse the patched CSV
	patchedContent, err := os.ReadFile(csvFile)
	if err != nil {
		t.Fatalf("Failed to read patched CSV: %v", err)
	}

	var csvData map[string]interface{}
	if err := yaml.Unmarshal(patchedContent, &csvData); err != nil {
		t.Fatalf("Failed to unmarshal patched CSV: %v", err)
	}

	// Navigate to env vars
	spec := csvData["spec"].(map[string]interface{})
	install := spec["install"].(map[string]interface{})
	installSpec := install["spec"].(map[string]interface{})
	deployments := installSpec["deployments"].([]interface{})
	deployment := deployments[0].(map[string]interface{})
	deploymentSpec := deployment["spec"].(map[string]interface{})
	template := deploymentSpec["template"].(map[string]interface{})
	podSpec := template["spec"].(map[string]interface{})
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	envVars := container["env"].([]interface{})

	// All three env vars should be patched to use scanner-v4:4.0.0-local
	expectedValue := "quay.io/rhacs-eng/scanner-v4:4.0.0-local"
	scannerV4EnvVars := []string{
		"RELATED_IMAGE_SCANNER_V4",
		"RELATED_IMAGE_SCANNER_V4_INDEXER",
		"RELATED_IMAGE_SCANNER_V4_MATCHER",
	}

	for _, envVar := range envVars {
		envMap := envVar.(map[string]interface{})
		name := envMap["name"].(string)

		for _, expectedName := range scannerV4EnvVars {
			if name == expectedName {
				value := envMap["value"].(string)
				if value != expectedValue {
					t.Errorf("%s not patched correctly. Got: %s, Expected: %s", name, value, expectedValue)
				}
			}
		}
	}
}

// TestPatchCSVWithLocalImages_AllLocalImages tests patching when all images are available locally
func TestPatchCSVWithLocalImages_AllLocalImages(t *testing.T) {
	// Create a temporary CSV file
	csvContent := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: rhacs-operator.v4.0.0
spec:
  install:
    spec:
      deployments:
      - name: rhacs-operator-controller-manager
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/rhacs-eng/stackrox-operator:4.0.0
                env:
                - name: RELATED_IMAGE_MAIN
                  value: quay.io/rhacs-eng/main:4.0.0
                - name: RELATED_IMAGE_SCANNER
                  value: quay.io/rhacs-eng/scanner:4.0.0
                - name: RELATED_IMAGE_SCANNER_DB
                  value: quay.io/rhacs-eng/scanner-db:4.0.0
                - name: RELATED_IMAGE_CENTRAL_DB
                  value: quay.io/rhacs-eng/central-db:4.0.0
                - name: RELATED_IMAGE_SCANNER_V4_DB
                  value: quay.io/rhacs-eng/scanner-v4-db:4.0.0
                - name: RELATED_IMAGE_SCANNER_V4
                  value: quay.io/rhacs-eng/scanner-v4-matcher:4.0.0
                - name: RELATED_IMAGE_COLLECTOR
                  value: quay.io/rhacs-eng/collector:4.0.0
                - name: OTHER_ENV
                  value: some-value
`

	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "test.csv.yaml")
	if err := os.WriteFile(csvFile, []byte(csvContent), 0644); err != nil {
		t.Fatalf("Failed to write test CSV: %v", err)
	}

	// Create local images map with all images
	localImages := map[string]string{
		"stackrox-operator:4.0.0": "quay.io/rhacs-eng/stackrox-operator:4.0.0",
		"main:4.0.0":              "quay.io/rhacs-eng/main:4.0.0",
		"scanner:4.0.0":           "quay.io/rhacs-eng/scanner:4.0.0",
		"scanner-db:4.0.0":        "quay.io/rhacs-eng/scanner-db:4.0.0",
		"central-db:4.0.0":        "quay.io/rhacs-eng/central-db:4.0.0",
		"scanner-v4-db:4.0.0":     "quay.io/rhacs-eng/scanner-v4-db:4.0.0",
		"scanner-v4:4.0.0":        "quay.io/rhacs-eng/scanner-v4:4.0.0",
	}

	// Patch the CSV
	err := patchCSVWithLocalImages(csvFile, "4.0.0", localImages)
	if err != nil {
		t.Fatalf("patchCSVWithLocalImages failed: %v", err)
	}

	// Read and parse the patched CSV
	patchedContent, err := os.ReadFile(csvFile)
	if err != nil {
		t.Fatalf("Failed to read patched CSV: %v", err)
	}

	var csvData map[string]interface{}
	if err := yaml.Unmarshal(patchedContent, &csvData); err != nil {
		t.Fatalf("Failed to unmarshal patched CSV: %v", err)
	}

	// Navigate to the container spec
	spec := csvData["spec"].(map[string]interface{})
	install := spec["install"].(map[string]interface{})
	installSpec := install["spec"].(map[string]interface{})
	deployments := installSpec["deployments"].([]interface{})
	deployment := deployments[0].(map[string]interface{})
	deploymentSpec := deployment["spec"].(map[string]interface{})
	template := deploymentSpec["template"].(map[string]interface{})
	podSpec := template["spec"].(map[string]interface{})
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})

	// Verify operator image was patched with quay.io path
	operatorImage := container["image"].(string)
	expectedOperatorImage := "quay.io/rhacs-eng/stackrox-operator:4.0.0"
	if operatorImage != expectedOperatorImage {
		t.Errorf("Operator image not patched correctly. Got: %s, Expected: %s", operatorImage, expectedOperatorImage)
	}

	// Verify RELATED_IMAGE_* env vars were patched with quay.io paths
	envVars := container["env"].([]interface{})
	expectedEnvVars := map[string]string{
		"RELATED_IMAGE_MAIN":          "quay.io/rhacs-eng/main:4.0.0",
		"RELATED_IMAGE_SCANNER":       "quay.io/rhacs-eng/scanner:4.0.0",
		"RELATED_IMAGE_SCANNER_DB":    "quay.io/rhacs-eng/scanner-db:4.0.0",
		"RELATED_IMAGE_CENTRAL_DB":    "quay.io/rhacs-eng/central-db:4.0.0",
		"RELATED_IMAGE_SCANNER_V4_DB": "quay.io/rhacs-eng/scanner-v4-db:4.0.0",
		"RELATED_IMAGE_SCANNER_V4":    "quay.io/rhacs-eng/scanner-v4:4.0.0",
		"RELATED_IMAGE_COLLECTOR":     "quay.io/rhacs-eng/collector:4.0.0", // Not in local images, stays unchanged
		"OTHER_ENV":                   "some-value",
	}

	for _, envVar := range envVars {
		envMap := envVar.(map[string]interface{})
		name := envMap["name"].(string)
		value := envMap["value"].(string)

		if expectedValue, ok := expectedEnvVars[name]; ok {
			if value != expectedValue {
				t.Errorf("Env var %s not patched correctly. Got: %s, Expected: %s", name, value, expectedValue)
			}
		}
	}
}

// TestPatchCSVWithLocalImages_PartialLocalImages tests patching when only some images are available locally
func TestPatchCSVWithLocalImages_PartialLocalImages(t *testing.T) {
	csvContent := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: rhacs-operator.v4.0.0
spec:
  install:
    spec:
      deployments:
      - name: rhacs-operator-controller-manager
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/rhacs-eng/stackrox-operator:4.0.0
                env:
                - name: RELATED_IMAGE_MAIN
                  value: quay.io/rhacs-eng/main:4.0.0
                - name: RELATED_IMAGE_SCANNER
                  value: quay.io/rhacs-eng/scanner:4.0.0
                - name: RELATED_IMAGE_SCANNER_DB
                  value: quay.io/rhacs-eng/scanner-db:4.0.0
`

	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "test.csv.yaml")
	if err := os.WriteFile(csvFile, []byte(csvContent), 0644); err != nil {
		t.Fatalf("Failed to write test CSV: %v", err)
	}

	// Only main and scanner are local
	localImages := map[string]string{
		"main:4.0.0":    "quay.io/rhacs-eng/main:4.0.0",
		"scanner:4.0.0": "quay.io/rhacs-eng/scanner:4.0.0",
	}

	err := patchCSVWithLocalImages(csvFile, "4.0.0", localImages)
	if err != nil {
		t.Fatalf("patchCSVWithLocalImages failed: %v", err)
	}

	// Read and parse the patched CSV
	patchedContent, err := os.ReadFile(csvFile)
	if err != nil {
		t.Fatalf("Failed to read patched CSV: %v", err)
	}

	var csvData map[string]interface{}
	if err := yaml.Unmarshal(patchedContent, &csvData); err != nil {
		t.Fatalf("Failed to unmarshal patched CSV: %v", err)
	}

	// Navigate to the container spec
	spec := csvData["spec"].(map[string]interface{})
	install := spec["install"].(map[string]interface{})
	installSpec := install["spec"].(map[string]interface{})
	deployments := installSpec["deployments"].([]interface{})
	deployment := deployments[0].(map[string]interface{})
	deploymentSpec := deployment["spec"].(map[string]interface{})
	template := deploymentSpec["template"].(map[string]interface{})
	podSpec := template["spec"].(map[string]interface{})
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})

	// Verify operator image was NOT patched (not in local images)
	operatorImage := container["image"].(string)
	expectedOperatorImage := "quay.io/rhacs-eng/stackrox-operator:4.0.0"
	if operatorImage != expectedOperatorImage {
		t.Errorf("Operator image should not be patched. Got: %s, Expected: %s", operatorImage, expectedOperatorImage)
	}

	// Verify only local images were patched with quay.io paths
	envVars := container["env"].([]interface{})
	expectedEnvVars := map[string]string{
		"RELATED_IMAGE_MAIN":       "quay.io/rhacs-eng/main:4.0.0",       // Patched with quay.io path
		"RELATED_IMAGE_SCANNER":    "quay.io/rhacs-eng/scanner:4.0.0",    // Patched with quay.io path
		"RELATED_IMAGE_SCANNER_DB": "quay.io/rhacs-eng/scanner-db:4.0.0", // Not patched, stays original
	}

	for _, envVar := range envVars {
		envMap := envVar.(map[string]interface{})
		name := envMap["name"].(string)
		value := envMap["value"].(string)

		if expectedValue, ok := expectedEnvVars[name]; ok {
			if value != expectedValue {
				t.Errorf("Env var %s incorrect. Got: %s, Expected: %s", name, value, expectedValue)
			}
		}
	}
}

// TestPatchCSVWithLocalImages_NoLocalImages tests skipping when no local images exist
func TestPatchCSVWithLocalImages_NoLocalImages(t *testing.T) {
	csvContent := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: rhacs-operator.v4.0.0
spec:
  install:
    spec:
      deployments:
      - name: rhacs-operator-controller-manager
        spec:
          template:
            spec:
              containers:
              - name: manager
                image: quay.io/rhacs-eng/stackrox-operator:4.0.0
                env:
                - name: RELATED_IMAGE_MAIN
                  value: quay.io/rhacs-eng/main:4.0.0
`

	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "test.csv.yaml")
	originalContent := []byte(csvContent)
	if err := os.WriteFile(csvFile, originalContent, 0644); err != nil {
		t.Fatalf("Failed to write test CSV: %v", err)
	}

	// Empty local images map
	localImages := map[string]string{}

	err := patchCSVWithLocalImages(csvFile, "4.0.0", localImages)
	if err != nil {
		t.Fatalf("patchCSVWithLocalImages failed: %v", err)
	}

	// Verify CSV was not modified
	patchedContent, err := os.ReadFile(csvFile)
	if err != nil {
		t.Fatalf("Failed to read patched CSV: %v", err)
	}

	var originalData, patchedData map[string]interface{}
	if err := yaml.Unmarshal(originalContent, &originalData); err != nil {
		t.Fatalf("Failed to unmarshal original CSV: %v", err)
	}
	if err := yaml.Unmarshal(patchedContent, &patchedData); err != nil {
		t.Fatalf("Failed to unmarshal patched CSV: %v", err)
	}

	// Navigate to the image field
	getImage := func(data map[string]interface{}) string {
		spec := data["spec"].(map[string]interface{})
		install := spec["install"].(map[string]interface{})
		installSpec := install["spec"].(map[string]interface{})
		deployments := installSpec["deployments"].([]interface{})
		deployment := deployments[0].(map[string]interface{})
		deploymentSpec := deployment["spec"].(map[string]interface{})
		template := deploymentSpec["template"].(map[string]interface{})
		podSpec := template["spec"].(map[string]interface{})
		containers := podSpec["containers"].([]interface{})
		container := containers[0].(map[string]interface{})
		return container["image"].(string)
	}

	originalImage := getImage(originalData)
	patchedImage := getImage(patchedData)

	if originalImage != patchedImage {
		t.Errorf("CSV should not be modified when no local images. Original: %s, Patched: %s", originalImage, patchedImage)
	}
}

// TestPatchCSVWithLocalImages_MalformedCSV tests error handling for malformed CSV
func TestPatchCSVWithLocalImages_MalformedCSV(t *testing.T) {
	csvContent := `this is not valid yaml: [[[`

	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "test.csv.yaml")
	if err := os.WriteFile(csvFile, []byte(csvContent), 0644); err != nil {
		t.Fatalf("Failed to write test CSV: %v", err)
	}

	localImages := map[string]string{
		"main:4.0.0": "quay.io/rhacs-eng/main:4.0.0",
	}

	err := patchCSVWithLocalImages(csvFile, "4.0.0", localImages)
	if err == nil {
		t.Error("Expected error for malformed CSV, got nil")
	}
}

// TestPatchCSVWithLocalImages_MissingFile tests error handling for missing CSV file
func TestPatchCSVWithLocalImages_MissingFile(t *testing.T) {
	localImages := map[string]string{
		"main:4.0.0": "quay.io/rhacs-eng/main:4.0.0",
	}

	err := patchCSVWithLocalImages("/nonexistent/file.yaml", "4.0.0", localImages)
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

// TestPatchCSVWithLocalImages_PreservesOtherContent tests that non-image content is preserved
func TestPatchCSVWithLocalImages_PreservesOtherContent(t *testing.T) {
	csvContent := `apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: rhacs-operator.v4.0.0
  labels:
    app: rhacs-operator
  annotations:
    description: "RHACS Operator"
spec:
  displayName: "RHACS Operator"
  install:
    spec:
      deployments:
      - name: rhacs-operator-controller-manager
        spec:
          replicas: 1
          template:
            metadata:
              labels:
                control-plane: controller-manager
            spec:
              containers:
              - name: manager
                image: quay.io/rhacs-eng/stackrox-operator:4.0.0
                resources:
                  limits:
                    cpu: 500m
                    memory: 128Mi
                env:
                - name: RELATED_IMAGE_MAIN
                  value: quay.io/rhacs-eng/main:4.0.0
                - name: OTHER_VAR
                  value: keep-me
`

	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "test.csv.yaml")
	if err := os.WriteFile(csvFile, []byte(csvContent), 0644); err != nil {
		t.Fatalf("Failed to write test CSV: %v", err)
	}

	localImages := map[string]string{
		"main:4.0.0": "quay.io/rhacs-eng/main:4.0.0",
	}

	err := patchCSVWithLocalImages(csvFile, "4.0.0", localImages)
	if err != nil {
		t.Fatalf("patchCSVWithLocalImages failed: %v", err)
	}

	// Read and parse the patched CSV
	patchedContent, err := os.ReadFile(csvFile)
	if err != nil {
		t.Fatalf("Failed to read patched CSV: %v", err)
	}

	var csvData map[string]interface{}
	if err := yaml.Unmarshal(patchedContent, &csvData); err != nil {
		t.Fatalf("Failed to unmarshal patched CSV: %v", err)
	}

	// Verify metadata is preserved
	metadata := csvData["metadata"].(map[string]interface{})
	if metadata["name"].(string) != "rhacs-operator.v4.0.0" {
		t.Error("Metadata name was not preserved")
	}
	labels := metadata["labels"].(map[string]interface{})
	if labels["app"].(string) != "rhacs-operator" {
		t.Error("Metadata labels were not preserved")
	}

	// Verify spec fields are preserved
	spec := csvData["spec"].(map[string]interface{})
	if spec["displayName"].(string) != "RHACS Operator" {
		t.Error("displayName was not preserved")
	}

	// Verify deployment details are preserved
	install := spec["install"].(map[string]interface{})
	installSpec := install["spec"].(map[string]interface{})
	deployments := installSpec["deployments"].([]interface{})
	deployment := deployments[0].(map[string]interface{})
	deploymentSpec := deployment["spec"].(map[string]interface{})

	if deploymentSpec["replicas"].(int) != 1 {
		t.Error("Replicas count was not preserved")
	}

	template := deploymentSpec["template"].(map[string]interface{})
	templateMeta := template["metadata"].(map[string]interface{})
	templateLabels := templateMeta["labels"].(map[string]interface{})
	if templateLabels["control-plane"].(string) != "controller-manager" {
		t.Error("Template labels were not preserved")
	}

	// Verify resources are preserved
	podSpec := template["spec"].(map[string]interface{})
	containers := podSpec["containers"].([]interface{})
	container := containers[0].(map[string]interface{})
	resources := container["resources"].(map[string]interface{})
	limits := resources["limits"].(map[string]interface{})
	if limits["cpu"].(string) != "500m" {
		t.Error("Resource limits were not preserved")
	}

	// Verify non-RELATED_IMAGE env vars are preserved
	envVars := container["env"].([]interface{})
	foundOtherVar := false
	for _, envVar := range envVars {
		envMap := envVar.(map[string]interface{})
		if envMap["name"].(string) == "OTHER_VAR" {
			if envMap["value"].(string) != "keep-me" {
				t.Error("OTHER_VAR value was modified")
			}
			foundOtherVar = true
		}
	}
	if !foundOtherVar {
		t.Error("OTHER_VAR was not preserved")
	}
}
