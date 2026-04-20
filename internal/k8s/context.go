package k8s

import (
	"os"
)

// detectKubectl returns the kubectl command to use.
func detectKubectl() string {
	if kubectl := os.Getenv("ORCH_CMD"); kubectl != "" {
		return kubectl
	}
	return "kubectl"
}

// GetKubectl returns the kubectl command to use.
func GetKubectl() string {
	return kubectl
}
