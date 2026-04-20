package k8s

import (
	"context"
	"testing"
)

func TestIsResourceNotFound(t *testing.T) {
	// Test that IsResourceNotFound correctly identifies the error
	if !IsResourceNotFound(ErrResourceNotFound) {
		t.Errorf("Expected IsResourceNotFound to return true for ErrResourceNotFound")
	}

	// Test with a different error
	otherErr := context.Canceled
	if IsResourceNotFound(otherErr) {
		t.Errorf("Expected IsResourceNotFound to return false for a different error")
	}
}
