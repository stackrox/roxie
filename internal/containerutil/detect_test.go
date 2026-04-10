package containerutil

import (
	"testing"
)

func TestIsRunningInRoxieContainer(t *testing.T) {
	// This test simply verifies the function doesn't panic
	// The actual result depends on whether the test is running
	// inside a container or not
	result := IsRunningInRoxieContainer()

	// Log the result for informational purposes
	t.Logf("IsRunningInRoxieContainer() = %v", result)

	// We can't assert a specific value since it depends on the environment
	// But we can verify it returns a boolean without errors
	if result {
		t.Log("Detected running inside a roxie container")
	} else {
		t.Log("Running outside of roxie container")
	}
}
