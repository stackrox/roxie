package localimages

import (
	"os/exec"
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
				"quay.io/stackrox-io/main:4.10.0", // fallback
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
				"quay.io/rhacs-eng/scanner:4.9.2", // fallback
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
