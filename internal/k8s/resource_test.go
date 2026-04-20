package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

func TestResourceNotOwnedByName(t *testing.T) {
	tests := []struct {
		name            string
		ownerReferences []metav1.OwnerReference
		ownerName       string
		expectedResult  bool
	}{
		{
			name:            "no owner references - not owned",
			ownerReferences: []metav1.OwnerReference{},
			ownerName:       "expected-owner",
			expectedResult:  true,
		},
		{
			name:            "nil owner references - not owned",
			ownerReferences: nil,
			ownerName:       "expected-owner",
			expectedResult:  true,
		},
		{
			name: "single owner reference - match",
			ownerReferences: []metav1.OwnerReference{
				{Name: "expected-owner", Kind: "Central"},
			},
			ownerName:      "expected-owner",
			expectedResult: false,
		},
		{
			name: "single owner reference - no match",
			ownerReferences: []metav1.OwnerReference{
				{Name: "different-owner", Kind: "Central"},
			},
			ownerName:      "expected-owner",
			expectedResult: true,
		},
		{
			name: "multiple owner references",
			ownerReferences: []metav1.OwnerReference{
				{Name: "expected-owner", Kind: "Central"},
				{Name: "other-owner", Kind: "SecuredCluster"},
			},
			ownerName:      "expected-owner",
			expectedResult: false,
		},
		{
			name: "multiple owner references - none match",
			ownerReferences: []metav1.OwnerReference{
				{Name: "owner-1", Kind: "Central"},
				{Name: "owner-2", Kind: "SecuredCluster"},
			},
			ownerName:      "expected-owner",
			expectedResult: true,
		},
		{
			name: "exact match with similar names",
			ownerReferences: []metav1.OwnerReference{
				{Name: "expected-owner-suffix", Kind: "Central"},
				{Name: "prefix-expected-owner", Kind: "Central"},
			},
			ownerName:      "expected-owner",
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{}
			obj.SetOwnerReferences(tt.ownerReferences)

			result := ResourceNotOwnedByName(obj, tt.ownerName)
			assert.Equal(t, tt.expectedResult, result, "ResourceNotOwnedByName() = %v, want %v", result, tt.expectedResult)
		})
	}
}
