package localimages

import (
	"context"
	"testing"

	"github.com/stackrox/roxie/internal/logger"
)

func TestLoadImageToKind(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// This would require a real kind cluster, so we just test the command building
	t.Log("Integration test placeholder - would require real kind cluster")
}

func TestLoadImagesToKind_EmptyImages(t *testing.T) {
	ctx := context.Background()
	// Create a test logger (use nil if logger.New doesn't exist in tests)
	log := &logger.Logger{}

	err := LoadImagesToKind(ctx, map[string]string{}, "test-cluster", log)
	if err != nil {
		t.Errorf("LoadImagesToKind with empty map should not error, got: %v", err)
	}
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
