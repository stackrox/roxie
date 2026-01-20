# Local Image Support for Kind Clusters - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Enable roxie to automatically detect and load locally-built container images from podman into kind clusters, eliminating the need to push to quay.io during local development.

**Architecture:** Add new `internal/localimages` and `internal/cluster` packages for image detection and kind cluster handling. Integrate into deployer workflow between cluster defaults and credential verification. Use existing `env` package's kind cluster detection. Support ROX_PRODUCT_BRANDING to check both localhost/stackrox and quay.io registry paths.

**Tech Stack:** Go 1.21+, podman CLI, kind CLI, kubectl, existing roxie packages (env, dockerauth, deployer)

---

## Task 1: Create branding support package

**Files:**
- Create: `internal/localimages/branding.go`
- Create: `internal/localimages/branding_test.go`

**Step 1: Write the failing test**

```go
package localimages

import (
	"testing"
)

func TestGetBrandingRegistry(t *testing.T) {
	tests := []struct {
		name            string
		brandingEnv     string
		expectedOrg     string
		expectedFlavor  string
	}{
		{
			name:           "RHACS branding",
			brandingEnv:    "RHACS_BRANDING",
			expectedOrg:    "rhacs-eng",
			expectedFlavor: "development_build",
		},
		{
			name:           "STACKROX branding",
			brandingEnv:    "STACKROX_BRANDING",
			expectedOrg:    "stackrox-io",
			expectedFlavor: "opensource",
		},
		{
			name:           "empty defaults to RHACS",
			brandingEnv:    "",
			expectedOrg:    "rhacs-eng",
			expectedFlavor: "development_build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ROX_PRODUCT_BRANDING", tt.brandingEnv)

			org := GetBrandingOrganization()
			if org != tt.expectedOrg {
				t.Errorf("GetBrandingOrganization() = %v, want %v", org, tt.expectedOrg)
			}

			flavor := GetImageFlavor()
			if flavor != tt.expectedFlavor {
				t.Errorf("GetImageFlavor() = %v, want %v", flavor, tt.expectedFlavor)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/localimages/... -v`
Expected: FAIL with "no buildable Go source files" or similar

**Step 3: Write minimal implementation**

```go
package localimages

import "os"

const (
	brandingEnvVar = "ROX_PRODUCT_BRANDING"

	rhacsBranding     = "RHACS_BRANDING"
	stackroxBranding  = "STACKROX_BRANDING"

	rhacsOrg     = "rhacs-eng"
	stackroxOrg  = "stackrox-io"

	rhacsFlavor     = "development_build"
	stackroxFlavor  = "opensource"
)

// GetBrandingOrganization returns the registry organization based on ROX_PRODUCT_BRANDING.
// Defaults to rhacs-eng if not set.
func GetBrandingOrganization() string {
	branding := os.Getenv(brandingEnvVar)
	if branding == stackroxBranding {
		return stackroxOrg
	}
	// Default to RHACS branding
	return rhacsOrg
}

// GetImageFlavor returns the image flavor based on ROX_PRODUCT_BRANDING.
// Defaults to development_build if not set.
func GetImageFlavor() string {
	branding := os.Getenv(brandingEnvVar)
	if branding == stackroxBranding {
		return stackroxFlavor
	}
	// Default to RHACS flavor
	return rhacsFlavor
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/localimages/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/localimages/branding.go internal/localimages/branding_test.go
git commit -m "Add branding support for local image detection

Implements ROX_PRODUCT_BRANDING environment variable support to determine
which registry organization to use (rhacs-eng vs stackrox-io). Defaults
to RHACS branding for typical development workflows.

Relates to #41"
```

---

## Task 2: Create kind cluster detection

**Files:**
- Create: `internal/cluster/kind.go`
- Create: `internal/cluster/kind_test.go`

**Step 1: Write the failing test**

```go
package cluster

import (
	"testing"
)

func TestIsKindCluster(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		expected    bool
	}{
		{
			name:        "kind cluster with prefix",
			contextName: "kind-acs",
			expected:    true,
		},
		{
			name:        "kind cluster just kind",
			contextName: "kind",
			expected:    true,
		},
		{
			name:        "non-kind cluster",
			contextName: "gke_project_zone_cluster",
			expected:    false,
		},
		{
			name:        "empty context",
			contextName: "",
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isKindContext(tt.contextName)
			if result != tt.expected {
				t.Errorf("isKindContext(%q) = %v, want %v", tt.contextName, result, tt.expected)
			}
		})
	}
}

func TestExtractKindClusterName(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		expected    string
	}{
		{
			name:        "kind with cluster name",
			contextName: "kind-acs",
			expected:    "acs",
		},
		{
			name:        "just kind",
			contextName: "kind",
			expected:    "kind",
		},
		{
			name:        "kind with dashes",
			contextName: "kind-my-cluster-name",
			expected:    "my-cluster-name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractKindClusterName(tt.contextName)
			if result != tt.expected {
				t.Errorf("ExtractKindClusterName(%q) = %v, want %v", tt.contextName, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cluster/... -v`
Expected: FAIL with "no buildable Go source files"

**Step 3: Write minimal implementation**

```go
package cluster

import (
	"strings"

	"github.com/stackrox/roxie/internal/env"
)

// IsKindCluster returns true if the current kubectl context is a kind cluster.
func IsKindCluster() bool {
	contextName := env.GetCurrentContext()
	return isKindContext(contextName)
}

// isKindContext checks if the given context name indicates a kind cluster.
// Exported for testing.
func isKindContext(contextName string) bool {
	if contextName == "" {
		return false
	}
	// Kind clusters have contexts starting with "kind" (case-insensitive)
	return strings.HasPrefix(strings.ToLower(contextName), "kind")
}

// ExtractKindClusterName extracts the cluster name from a kind context.
// For context "kind-acs", returns "acs".
// For context "kind", returns "kind".
func ExtractKindClusterName(contextName string) string {
	// Remove "kind-" prefix if present
	if len(contextName) > 5 && strings.HasPrefix(strings.ToLower(contextName), "kind-") {
		return contextName[5:]
	}
	// If just "kind", return as-is
	return contextName
}

// GetKindClusterName returns the kind cluster name for the current context.
// Returns empty string if not a kind cluster.
func GetKindClusterName() string {
	if !IsKindCluster() {
		return ""
	}
	return ExtractKindClusterName(env.GetCurrentContext())
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cluster/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/cluster/kind.go internal/cluster/kind_test.go
git commit -m "Add kind cluster detection utilities

Implements detection of kind clusters based on kubectl context name.
Extracts cluster name for use with 'kind load' commands.

Relates to #41"
```

---

## Task 3: Create local image detection

**Files:**
- Create: `internal/localimages/detection.go`
- Create: `internal/localimages/detection_test.go`

**Step 1: Write the failing test**

```go
package localimages

import (
	"testing"
)

func TestBuildImageReferences(t *testing.T) {
	tests := []struct {
		name      string
		imageName string
		tag       string
		branding  string
		expected  []string
	}{
		{
			name:      "RHACS branding",
			imageName: "main",
			tag:       "4.10.0",
			branding:  "RHACS_BRANDING",
			expected: []string{
				"localhost/stackrox/main:4.10.0",
				"quay.io/rhacs-eng/main:4.10.0",
			},
		},
		{
			name:      "STACKROX branding",
			imageName: "scanner",
			tag:       "4.9.2",
			branding:  "STACKROX_BRANDING",
			expected: []string{
				"localhost/stackrox/scanner:4.9.2",
				"quay.io/stackrox-io/scanner:4.9.2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ROX_PRODUCT_BRANDING", tt.branding)

			result := buildImageReferences(tt.imageName, tt.tag)
			if len(result) != len(tt.expected) {
				t.Fatalf("buildImageReferences() returned %d refs, want %d", len(result), len(tt.expected))
			}
			for i, ref := range result {
				if ref != tt.expected[i] {
					t.Errorf("buildImageReferences()[%d] = %v, want %v", i, ref, tt.expected[i])
				}
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/localimages/... -v -run TestBuild`
Expected: FAIL with "undefined: buildImageReferences"

**Step 3: Write minimal implementation**

```go
package localimages

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	localhostPrefix = "localhost/stackrox"
	quayRegistry    = "quay.io"
)

// buildImageReferences returns candidate image references to check in podman.
// Returns in priority order: localhost/stackrox first, then quay.io registry.
func buildImageReferences(imageName, tag string) []string {
	org := GetBrandingOrganization()
	return []string{
		fmt.Sprintf("%s/%s:%s", localhostPrefix, imageName, tag),
		fmt.Sprintf("%s/%s/%s:%s", quayRegistry, org, imageName, tag),
	}
}

// CheckLocalImage checks if an image exists in podman.
// Returns the full image reference if found, empty string if not found.
func CheckLocalImage(imageName, tag string) (string, error) {
	refs := buildImageReferences(imageName, tag)

	for _, ref := range refs {
		exists, err := podmanImageExists(ref)
		if err != nil {
			return "", fmt.Errorf("checking podman for %s: %w", ref, err)
		}
		if exists {
			return ref, nil
		}
	}

	return "", nil
}

// podmanImageExists checks if an image exists in podman using 'podman image exists'.
// Returns true if the image exists (exit code 0), false otherwise.
func podmanImageExists(imageRef string) (bool, error) {
	cmd := exec.Command("podman", "image", "exists", imageRef)
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means image doesn't exist (expected)
		// Other errors are actual failures
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ImageSet represents a collection of images to check.
type ImageSet struct {
	Main      string
	Operator  string
	Images    []string
}

// CheckImages checks which images from the set exist locally.
// Returns a map of image names to their full references.
func CheckImages(mainTag, operatorTag string) (map[string]string, error) {
	images := []string{
		"main",
		"scanner",
		"scanner-db",
		"scanner-v4-db",
		"scanner-v4-indexer",
		"scanner-v4-matcher",
		"central-db",
	}

	operatorImages := []string{
		"stackrox-operator-bundle",
		"stackrox-operator-index",
	}

	localImages := make(map[string]string)

	// Check main images
	for _, imageName := range images {
		ref, err := CheckLocalImage(imageName, mainTag)
		if err != nil {
			return nil, err
		}
		if ref != "" {
			localImages[imageName+":"+mainTag] = ref
		}
	}

	// Check operator images with v prefix
	for _, imageName := range operatorImages {
		ref, err := CheckLocalImage(imageName, "v"+operatorTag)
		if err != nil {
			return nil, err
		}
		if ref != "" {
			localImages[imageName+":v"+operatorTag] = ref
		}
	}

	return localImages, nil
}
```

**Step 4: Add integration test (requires podman)**

```go
// Add to detection_test.go
func TestCheckLocalImage_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// This test requires podman to be available
	cmd := exec.Command("podman", "version")
	if err := cmd.Run(); err != nil {
		t.Skip("podman not available, skipping integration test")
	}

	// Try to find a known image (alpine is commonly available)
	// This is just to test the mechanism works
	t.Log("Integration test - checking if podman command works")
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./internal/localimages/... -v -short`
Expected: PASS (skips integration test)

**Step 6: Commit**

```bash
git add internal/localimages/detection.go internal/localimages/detection_test.go
git commit -m "Add local image detection for podman

Implements image existence checking in podman with branding-aware paths.
Checks localhost/stackrox and quay.io registry prefixes in priority order.
Supports all ACS images (main, scanner, operator, etc.).

Relates to #41"
```

---

## Task 4: Create kind image loading

**Files:**
- Create: `internal/localimages/loading.go`
- Create: `internal/localimages/loading_test.go`

**Step 1: Write the failing test**

```go
package localimages

import (
	"context"
	"testing"
)

func TestLoadImageToKind(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// This would require a real kind cluster, so we just test the command building
	t.Log("Integration test placeholder - would require real kind cluster")
}

func TestBuildKindLoadCommand(t *testing.T) {
	tests := []struct {
		name        string
		imageRef    string
		clusterName string
		expected    []string
	}{
		{
			name:        "basic load",
			imageRef:    "localhost/stackrox/main:4.10.0",
			clusterName: "acs",
			expected:    []string{"kind", "load", "docker-image", "localhost/stackrox/main:4.10.0", "-n", "acs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildKindLoadCommand(tt.imageRef, tt.clusterName)
			if len(cmd) != len(tt.expected) {
				t.Fatalf("command length = %d, want %d", len(cmd), len(tt.expected))
			}
			for i, arg := range cmd {
				if arg != tt.expected[i] {
					t.Errorf("cmd[%d] = %q, want %q", i, arg, tt.expected[i])
				}
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/localimages/... -v -short -run TestBuild`
Expected: FAIL with "undefined: buildKindLoadCommand"

**Step 3: Write minimal implementation**

```go
// Add to loading.go
package localimages

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/stackrox/roxie/internal/logger"
)

// LoadImageToKind loads a single image into a kind cluster.
func LoadImageToKind(ctx context.Context, imageRef, clusterName string, log *logger.Logger) error {
	log.Dimf("Loading %s into kind cluster %s", imageRef, clusterName)

	cmd := exec.CommandContext(ctx, "kind", "load", "docker-image", imageRef, "-n", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("kind load failed for %s: %w\nOutput: %s", imageRef, err, string(output))
	}

	return nil
}

// buildKindLoadCommand builds the kind load command arguments.
func buildKindLoadCommand(imageRef, clusterName string) []string {
	return []string{"kind", "load", "docker-image", imageRef, "-n", clusterName}
}

// LoadImagesToKind loads multiple images into a kind cluster in parallel.
// Uses up to 4 concurrent workers to speed up loading.
func LoadImagesToKind(ctx context.Context, images map[string]string, clusterName string, log *logger.Logger) error {
	if len(images) == 0 {
		return nil
	}

	log.Infof("Loading %d images into kind cluster %s", len(images), clusterName)

	// Channel for images to process
	imageChan := make(chan string, len(images))
	for _, imageRef := range images {
		imageChan <- imageRef
	}
	close(imageChan)

	// Error channel
	errChan := make(chan error, len(images))

	// Use 4 workers for parallel loading (matching existing image verification parallelism)
	const numWorkers = 4
	var wg sync.WaitGroup

	for i := 0; i < numWorkers && i < len(images); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for imageRef := range imageChan {
				if err := LoadImageToKind(ctx, imageRef, clusterName, log); err != nil {
					errChan <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	if err := <-errChan; err != nil {
		return err
	}

	log.Infof("Successfully loaded %d images into kind cluster", len(images))
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/localimages/... -v -short`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/localimages/loading.go internal/localimages/loading_test.go
git commit -m "Add kind image loading functionality

Implements parallel loading of container images from podman into kind
clusters using 'kind load docker-image'. Uses 4 concurrent workers
for performance.

Relates to #41"
```

---

## Task 5: Integrate into deployer

**Files:**
- Modify: `internal/deployer/deployer.go`

**Step 1: Add local image fields to Deployer struct**

Locate the `Deployer` struct (around line 89) and add new fields:

```go
type Deployer struct {
	logger                 *logger.Logger
	startTime              time.Time
	dockerAuth             *dockerauth.DockerAuth
	imageCache             *imagecache.ImageCache
	portForward            *portforward.Manager
	clusterDefaults        *clusterdefaults.Manager
	kubectl                string
	roxctlVersion          string
	centralNamespace       string
	sensorNamespace        string
	mainImageTag           string
	operatorTag            string
	centralEndpoint        string
	centralPassword        string
	roxCACertFile          string
	kubeContext            string
	portForwardEnabled     bool
	pauseReconciliation    bool
	exposure               string
	overrideFile           string
	overrideSetExpressions []string
	envrcFile              string
	useHelm                bool
	useOLM                 bool
	shouldDeployOperator   bool
	verbose                bool
	earlyReadiness         bool
	dockerCreds            *dockerauth.Credentials
	clusterResourceKinds   map[string]struct{}
	// New fields for local image support
	localImages            map[string]string  // map of image names to local references
	usingLocalImages       bool               // true if any local images were found and loaded
}
```

**Step 2: Add import statements**

At the top of `deployer.go`, add new imports:

```go
import (
	// ... existing imports ...
	"github.com/stackrox/roxie/internal/cluster"
	"github.com/stackrox/roxie/internal/localimages"
)
```

**Step 3: Add local image detection and loading method**

Add new method after `prepareCredentials()` (around line 420):

```go
// detectAndLoadLocalImages checks for local images and loads them into kind if applicable.
// Returns true if local images were found and loaded, false otherwise.
func (d *Deployer) detectAndLoadLocalImages(ctx context.Context) error {
	// Check if ROXIE_SKIP_LOCAL_IMAGES is set
	if os.Getenv("ROXIE_SKIP_LOCAL_IMAGES") == "true" {
		d.logger.Dim("ROXIE_SKIP_LOCAL_IMAGES is set, skipping local image detection")
		return nil
	}

	// Check if this is a kind cluster
	if !cluster.IsKindCluster() {
		d.logger.Dim("Not a kind cluster, skipping local image detection")
		return nil
	}

	kindClusterName := cluster.GetKindClusterName()
	d.logger.Infof("Detected kind cluster: %s", kindClusterName)

	// Check for local images
	d.logger.Dim("Checking for local images in podman...")
	localImages, err := localimages.CheckImages(d.mainImageTag, d.operatorTag)
	if err != nil {
		// If podman is not available, gracefully fall back
		d.logger.Dimf("Could not check for local images: %v", err)
		d.logger.Dim("Falling back to quay.io")
		return nil
	}

	if len(localImages) == 0 {
		d.logger.Dim("No local images found, will pull from quay.io")
		return nil
	}

	// Calculate total images needed (7 main + 2 operator = 9)
	totalExpected := 9
	d.logger.Infof("Found %d/%d images locally in podman", len(localImages), totalExpected)

	// Load images into kind
	if err := localimages.LoadImagesToKind(ctx, localImages, kindClusterName, d.logger); err != nil {
		return fmt.Errorf("failed to load images into kind cluster: %w", err)
	}

	// Store the local images for later use
	d.localImages = localImages
	d.usingLocalImages = len(localImages) > 0

	return nil
}
```

**Step 4: Modify Deploy method to call local image detection**

Locate the `Deploy` method (around line 371) and add the call after cluster defaults, before credentials:

```go
func (d *Deployer) Deploy(ctx context.Context, component, resources, exposure string) error {
	adjustedResources, adjustedExposure, adjustedPortForward := d.clusterDefaults.ApplyConvenienceDefaults(
		d.kubeContext,
		resources,
		exposure,
		d.portForwardEnabled,
	)

	resources = adjustedResources
	exposure = adjustedExposure
	d.portForwardEnabled = adjustedPortForward
	d.exposure = exposure

	// NEW: Detect and load local images for kind clusters
	if err := d.detectAndLoadLocalImages(ctx); err != nil {
		return fmt.Errorf("failed to detect and load local images: %w", err)
	}

	// Prepare and verify credentials early to fail fast
	// Skip if all images are local
	if !d.shouldSkipCredentialVerification() {
		if err := d.prepareCredentials(); err != nil {
			return fmt.Errorf("failed to prepare credentials: %w", err)
		}
	} else {
		d.logger.Info("All images loaded locally, skipping credential verification")
	}

	d.logger.Infof("Initiating deployment of %s", formatComponentName(component))

	switch component {
	case "central":
		return d.deployCentral(ctx, resources, exposure)
	case "secured-cluster", "sensor":
		return d.deploySecuredCluster(ctx, resources)
	case "both", "all":
		if err := d.deployCentral(ctx, resources, exposure); err != nil {
			return fmt.Errorf("failed to deploy central: %w", err)
		}
		return d.deploySecuredCluster(ctx, resources)
	default:
		return fmt.Errorf("unknown component: %s", component)
	}
}
```

**Step 5: Add helper method to determine if credentials should be skipped**

```go
// shouldSkipCredentialVerification returns true if we should skip credential verification.
// We skip if all required images are available locally.
func (d *Deployer) shouldSkipCredentialVerification() bool {
	// If not using any local images, don't skip
	if !d.usingLocalImages {
		return false
	}

	// If using some local images but not all, don't skip (need creds for remote pulls)
	// Total expected: 7 main + 2 operator = 9
	totalExpected := 9
	if len(d.localImages) < totalExpected {
		d.logger.Dimf("Using %d/%d local images, remaining images will be pulled from quay.io",
			len(d.localImages), totalExpected)
		return false
	}

	// All images are local
	return true
}
```

**Step 6: Build and test**

Run: `go build ./cmd/roxie`
Expected: Build succeeds

**Step 7: Commit**

```bash
git add internal/deployer/deployer.go
git commit -m "Integrate local image detection into deployment flow

Adds automatic detection and loading of local images for kind clusters.
Checks podman for images before deployment and loads them into kind.
Skips credential verification when all images are available locally.

Supports ROXIE_SKIP_LOCAL_IMAGES environment variable to disable.

Relates to #41"
```

---

## Task 6: Handle image pull secrets for partial local images

**Files:**
- Modify: `internal/deployer/operator.go`
- Modify: `internal/deployer/deploy_via_helm.go`

**Step 1: Review current pull secret creation**

Read `internal/deployer/operator.go` to find where image pull secrets are created. Look for `createImagePullSecret` or similar methods.

**Step 2: Add conditional pull secret creation**

Modify the pull secret creation to skip when all images are local:

```go
// In the deployment method, wrap pull secret creation:
if !d.shouldSkipImagePullSecrets() {
	if err := d.createImagePullSecret(ctx, namespace); err != nil {
		return fmt.Errorf("failed to create image pull secret: %w", err)
	}
} else {
	d.logger.Dim("All images loaded locally, skipping image pull secret creation")
}
```

**Step 3: Add helper method**

Add to `deployer.go`:

```go
// shouldSkipImagePullSecrets returns true if image pull secrets should be skipped.
// Same logic as credential verification - skip only if all images are local.
func (d *Deployer) shouldSkipImagePullSecrets() bool {
	return d.shouldSkipCredentialVerification()
}
```

**Step 4: Update helm deployment similarly**

Apply same logic in `deploy_via_helm.go` if it creates pull secrets.

**Step 5: Build and test**

Run: `go build ./cmd/roxie`
Expected: Build succeeds

**Step 6: Commit**

```bash
git add internal/deployer/operator.go internal/deployer/deploy_via_helm.go internal/deployer/deployer.go
git commit -m "Skip image pull secrets when all images are local

When all required images are loaded locally into kind, skip creating
image pull secrets since they're not needed. Still creates secrets
for partial local image scenarios.

Relates to #41"
```

---

## Task 7: Add environment variable documentation

**Files:**
- Modify: `README.md` or create `docs/local-images.md`

**Step 1: Document the feature**

Add documentation for the new feature (location depends on existing docs structure):

````markdown
## Local Image Support for Kind Clusters

Roxie automatically detects and uses locally-built container images when deploying to kind clusters, eliminating the need to push images to quay.io during development.

### How It Works

When deploying to a kind cluster, roxie:

1. Checks if images exist locally in podman
2. Loads found images into the kind cluster
3. Skips credential verification if all images are local
4. Falls back to quay.io for any missing images

### Requirements

- kind cluster (context name must start with "kind")
- podman with images built locally
- Images tagged with either:
  - `localhost/stackrox/<image>:<tag>`
  - `quay.io/<branding-org>/<image>:<tag>`

### Supported Images

- main
- scanner, scanner-db
- scanner-v4-db, scanner-v4-indexer, scanner-v4-matcher
- central-db
- stackrox-operator-bundle
- stackrox-operator-index

### Environment Variables

**ROX_PRODUCT_BRANDING**: Controls which registry organization to check
- `RHACS_BRANDING` → `quay.io/rhacs-eng` (default)
- `STACKROX_BRANDING` → `quay.io/stackrox-io`

**ROXIE_SKIP_LOCAL_IMAGES**: Set to `true` to disable local image detection and force quay.io pulls

### Example Workflow

```bash
# Build stackrox locally (images go to podman)
cd /path/to/stackrox
make image

# Deploy to kind - roxie automatically uses local images
cd /path/to/roxie
./roxie deploy both
```

### Behavior

- **All images local**: Skips credential verification, fast deployment
- **Some images local**: Loads local ones, pulls remaining from quay.io
- **No images local**: Normal quay.io workflow (backward compatible)
- **Non-kind cluster**: Skips local image detection entirely
````

**Step 2: Commit**

```bash
git add README.md  # or docs/local-images.md
git commit -m "Add documentation for local image support

Documents the automatic local image detection and loading feature
for kind clusters, including requirements and usage examples.

Relates to #41"
```

---

## Task 8: Manual testing

**Testing Checklist:**

Run these manual tests to verify the implementation:

**Test 1: All images local (happy path)**
```bash
# Prerequisite: Build stackrox locally with RHACS branding
cd /path/to/stackrox
make image
podman images | grep stackrox  # Verify images exist

# Test: Deploy to kind
cd /path/to/roxie
./roxie deploy both

# Expected:
# - "Detected kind cluster: acs"
# - "Found 9/9 images locally in podman"
# - "Loading 9 images into kind cluster..."
# - "All images loaded locally, skipping credential verification"
# - Deployment succeeds
```

**Test 2: Partial local images**
```bash
# Delete some images to simulate partial scenario
podman rmi localhost/stackrox/scanner:TAG

# Test: Deploy to kind
./roxie deploy both

# Expected:
# - "Found 8/9 images locally in podman"
# - "Using 8/9 local images, remaining images will be pulled from quay.io"
# - Credential verification still runs
# - Deployment succeeds (pulls missing image from quay.io)
```

**Test 3: No local images**
```bash
# Delete all local images
podman rmi localhost/stackrox/main:TAG  # (and others)

# Test: Deploy to kind
./roxie deploy both

# Expected:
# - "No local images found, will pull from quay.io"
# - Normal credential verification
# - Deployment succeeds
```

**Test 4: Non-kind cluster**
```bash
# Switch to non-kind cluster
kubectl config use-context gke_project_zone_cluster

# Test: Deploy
./roxie deploy central

# Expected:
# - "Not a kind cluster, skipping local image detection"
# - Normal quay.io workflow
# - No kind-specific behavior
```

**Test 5: STACKROX branding**
```bash
# Build with STACKROX branding
cd /path/to/stackrox
ROX_PRODUCT_BRANDING=STACKROX_BRANDING make image

# Test: Deploy
cd /path/to/roxie
ROX_PRODUCT_BRANDING=STACKROX_BRANDING ./roxie deploy both

# Expected:
# - Uses quay.io/stackrox-io paths
# - Finds images correctly
```

**Test 6: Skip local images**
```bash
# Test with override
ROXIE_SKIP_LOCAL_IMAGES=true ./roxie deploy both

# Expected:
# - "ROXIE_SKIP_LOCAL_IMAGES is set, skipping local image detection"
# - Normal quay.io workflow
```

**Test 7: Image loading failure**
```bash
# This is hard to test without breaking kind, but verify error handling:
# - Corrupt an image
# - Expect: Clear error message, deployment aborted
```

**Mark complete when all tests pass**

No commit needed - this is verification only.

---

## Task 9: Update CHANGELOG (if exists)

**Files:**
- Modify: `CHANGELOG.md` (if it exists)

**Step 1: Check if CHANGELOG exists**

Run: `ls -la CHANGELOG.md`

**Step 2: If exists, add entry**

```markdown
## [Unreleased]

### Added
- Automatic local image detection and loading for kind clusters (#41)
  - Checks podman for locally-built images before deployment
  - Automatically loads images into kind cluster using `kind load`
  - Skips credential verification when all images available locally
  - Supports ROX_PRODUCT_BRANDING for RHACS/STACKROX brandings
  - Gracefully falls back to quay.io for missing images
  - Can be disabled with ROXIE_SKIP_LOCAL_IMAGES=true
```

**Step 3: Commit if updated**

```bash
git add CHANGELOG.md
git commit -m "Update CHANGELOG for local image support

Relates to #41"
```

---

## Testing Strategy Summary

**Unit Tests**: Tasks 1-4 include unit tests for individual components
**Integration Points**: Task 5 integrates all components
**Manual Testing**: Task 8 provides comprehensive manual test scenarios

**Key Test Scenarios**:
1. All local images → skip credentials
2. Partial local images → load local + pull remote
3. No local images → normal quay.io flow
4. Non-kind cluster → skip local detection
5. Different brandings → correct registry paths
6. Override flag → force quay.io
7. Error cases → clear error messages

**Success Criteria**:
- All unit tests pass
- Build succeeds without errors
- Manual tests verify expected behavior
- Backward compatible with non-kind deployments
- Clear user feedback throughout process

---

## Implementation Notes

**Code Style**: Follow existing roxie conventions (logger usage, error handling)
**Error Handling**: Fail fast on critical errors (kind load failures), gracefully degrade on non-critical (podman unavailable)
**Logging**: Use logger.Info for user-visible actions, logger.Dim for technical details
**Concurrency**: Use 4 workers for parallel image loading (matches existing imagecache pattern)
**Testing**: Use t.Skip for integration tests that require external tools

**Dependencies**: No new external Go modules required, uses existing:
- os/exec for CLI commands
- existing roxie packages (env, logger, dockerauth, deployer)
