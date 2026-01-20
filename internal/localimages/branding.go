package localimages

import "os"

const (
	brandingEnvVar = "ROX_PRODUCT_BRANDING"

	rhacsBranding     = "RHACS_BRANDING"
	stackroxBranding  = "STACKROX_BRANDING"

	rhacsOrg     = "rhacs-eng"
	stackroxOrg  = "stackrox-io"

	rhacsFlavor     = "development_build"
	stackroxFlavor  = "opensource"
)

// GetBrandingOrganization returns the registry organization based on ROX_PRODUCT_BRANDING.
// Defaults to rhacs-eng if not set.
func GetBrandingOrganization() string {
	branding := os.Getenv(brandingEnvVar)
	if branding == stackroxBranding {
		return stackroxOrg
	}
	// Default to RHACS branding
	return rhacsOrg
}

// GetImageFlavor returns the image flavor based on ROX_PRODUCT_BRANDING.
// Defaults to development_build if not set.
func GetImageFlavor() string {
	branding := os.Getenv(brandingEnvVar)
	if branding == stackroxBranding {
		return stackroxFlavor
	}
	// Default to RHACS flavor
	return rhacsFlavor
}
