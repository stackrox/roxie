package localimages

import (
	"testing"
)

func TestGetBrandingRegistry(t *testing.T) {
	tests := []struct {
		name            string
		brandingEnv     string
		expectedOrg     string
		expectedFlavor  string
	}{
		{
			name:           "RHACS branding",
			brandingEnv:    "RHACS_BRANDING",
			expectedOrg:    "rhacs-eng",
			expectedFlavor: "development_build",
		},
		{
			name:           "STACKROX branding",
			brandingEnv:    "STACKROX_BRANDING",
			expectedOrg:    "stackrox-io",
			expectedFlavor: "opensource",
		},
		{
			name:           "empty defaults to RHACS",
			brandingEnv:    "",
			expectedOrg:    "rhacs-eng",
			expectedFlavor: "development_build",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ROX_PRODUCT_BRANDING", tt.brandingEnv)

			org := GetBrandingOrganization()
			if org != tt.expectedOrg {
				t.Errorf("GetBrandingOrganization() = %v, want %v", org, tt.expectedOrg)
			}

			flavor := GetImageFlavor()
			if flavor != tt.expectedFlavor {
				t.Errorf("GetImageFlavor() = %v, want %v", flavor, tt.expectedFlavor)
			}
		})
	}
}
