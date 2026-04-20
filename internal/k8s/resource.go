package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/stackrox/roxie/internal/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	// ErrResourceNotFound is returned when a resource is not found in the cluster.
	ErrResourceNotFound = errors.New("resource not found")
)

// RetrieveResourceFromCluster retrieves a Kubernetes resource from the cluster and returns it as an unstructured.Unstructured.
//
// Parameters:
//   - ctx: The context for the kubectl command
//   - log: Logger for diagnostic output (can be nil for silent operation)
//   - namespace: The namespace where the resource is located (use "" for cluster-scoped resources)
//   - resourceType: The resource type (e.g., "pod", "secret", "pvc")
//   - resourceName: The name of the resource
//
// Returns:
//   - *unstructured.Unstructured: The resource as an unstructured object containing all metadata
//   - error: nil if successful, error otherwise (including ErrResourceNotFound for not found resources)
func RetrieveResourceFromCluster(ctx context.Context, log *logger.Logger, namespace, resourceType, resourceName string) (*unstructured.Unstructured, error) {
	// We use --ignore-not-found=true for more reliable distinction between "not found" and other errors.
	args := []string{"get", resourceType, resourceName, "-o", "json", "--ignore-not-found=true"}
	if namespace != "" {
		args = append([]string{"-n", namespace}, args...)
	}

	result, err := RunKubectl(ctx, log, KubectlOptions{
		Args: args,
	})

	// When --ignore-not-found=true is used, kubectl returns success (err == nil) with empty output if not found,
	// which seems more reliable than parsing error messages.
	if err == nil && len(strings.TrimSpace(result.Stdout)) == 0 {
		return nil, ErrResourceNotFound
	}
	if err != nil {
		if log != nil {
			log.Warningf("Failed to retrieve %s/%s from namespace %s: %v", resourceType, resourceName, namespace, err)
		}
		return nil, fmt.Errorf("kubectl get failed: %w", err)
	}

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(result.Stdout), obj); err != nil {
		if log != nil {
			log.Warningf("Failed to unmarshal %s/%s: %v", resourceType, resourceName, err)
		}
		return nil, fmt.Errorf("failed to unmarshal resource JSON: %w", err)
	}

	return obj, nil
}

// IsResourceNotFound checks if an error is a resource not found error.
func IsResourceNotFound(err error) bool {
	return err == ErrResourceNotFound
}

// ResourceNotOwnedByName checks if the given unstructured object does not have an owner reference with the specified owner name.
// The negative version of this function is better suited for our needs, because there can be multiple owner references and
// we are only interested in checking if a given resource is NOT owned by a specific owner.
func ResourceNotOwnedByName(obj *unstructured.Unstructured, ownerName string) bool {
	ownerRefs := obj.GetOwnerReferences()

	for _, ownerRef := range ownerRefs {
		if ownerRef.Name == ownerName {
			return false
		}
	}
	return true
}
