package env_test

import (
	"fmt"

	"github.com/stackrox/roxie-golang/pkg/env"
)

// ExampleDetectClusterType demonstrates how to use the cluster type detection
func ExampleDetectClusterType() {
	// Detect the cluster type for the current kubectl context
	clusterType := env.DetectClusterType()
	fmt.Printf("Detected cluster type: %s\n", clusterType)
}
