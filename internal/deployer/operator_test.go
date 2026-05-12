package deployer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetOperatorBundleImage(t *testing.T) {
	tests := []struct {
		name      string
		tag       string
		konflux   bool
		wantImage string
	}{
		{
			name:      "non-konflux",
			tag:       "4.6.0",
			konflux:   false,
			wantImage: "quay.io/rhacs-eng/stackrox-operator-bundle:v4.6.0",
		},
		{
			name:      "konflux appends -fast",
			tag:       "4.6.0",
			konflux:   true,
			wantImage: "quay.io/rhacs-eng/release-operator-bundle:v4.6.0-fast",
		},
		{
			name:      "konflux with existing -fast suffix",
			tag:       "4.6.0-fast",
			konflux:   true,
			wantImage: "quay.io/rhacs-eng/release-operator-bundle:v4.6.0-fast",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getOperatorBundleImage(tt.tag, tt.konflux)
			assert.Equal(t, tt.wantImage, got, "operator image mismatch")
		})
	}
}
