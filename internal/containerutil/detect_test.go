package containerutil

import (
	"testing"
)

func TestIsRunningInContainer(t *testing.T) {
	// This test simply verifies the function doesn't panic
	// The actual result depends on whether the test is running
	// inside a container or not
	result := IsRunningInContainer()

	// Log the result for informational purposes
	t.Logf("IsRunningInContainer() = %v", result)

	// We can't assert a specific value since it depends on the environment
	// But we can verify it returns a boolean without errors
	if result {
		t.Log("Detected running inside a container")
	} else {
		t.Log("Detected running on host (not in container)")
	}
}
