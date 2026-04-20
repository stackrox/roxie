//go:build integration

package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetrieveResourceFromCluster_NotFound(t *testing.T) {
	ctx := context.Background()
	namespace := "default"
	resourceType := "pod"
	resourceName := "i-do-not-exist-0987654321"

	obj, err := RetrieveResourceFromCluster(ctx, nil, namespace, resourceType, resourceName)
	assert.Nil(t, obj, "Expected no object to be returned when resource is not found")
	assert.ErrorIs(t, err, ErrResourceNotFound, "Expected ErrResourceNotFound when resource is not found")
}

func TestRetrieveResourceFromCluster_Found(t *testing.T) {
	ctx := context.Background()

	obj, err := RetrieveResourceFromCluster(ctx, nil, "", "namespace", "default")
	assert.NotNil(t, obj, "Expected an object to be returned when resource is found")
	assert.Nil(t, err, "Expected no error to be returned when resource is found")
	assert.Equal(t, "default", obj.GetName(), "Expected resource name to match")
}
