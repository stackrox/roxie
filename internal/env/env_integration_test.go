//go:build integration

package env

import (
	"testing"

	"github.com/stackrox/roxie/internal/types"
)

func TestDetectClusterType_Integration(t *testing.T) {
	err := Initialize(nil)
	if err != nil {
		t.Fatalf("Initialize() failed: %v", err)
	}

	// This test uses the current kubectl context
	// The result will depend on the active cluster
	clusterType := GetAutoDetectedClusterType()

	t.Logf("Detected cluster type: %s", clusterType)

	// The cluster type should never be invalid (even if Unknown)
	validTypes := types.AllClusterTypes()
	found := false
	for _, valid := range validTypes {
		if clusterType == valid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectClusterType() returned invalid type: %v", clusterType)
	}
}
