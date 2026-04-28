package stackroxversions

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

var (
	SupportsAdditionalPrinterColumnsConstraint = func() *semver.Constraints {
		constraint, err := semver.NewConstraint(">= 4.9.0")
		if err != nil {
			panic("invalid semver constraint")
		}
		return constraint
	}()
)

// SupportsAdditionalPrinterColumns checks if the provided main image tag supports
// the additional printer columns (i.e., >= 4.9.0).
func SupportsAdditionalPrinterColumns(version string) (bool, error) {
	semVer, err := semver.NewVersion(version)
	if err != nil {
		return false, fmt.Errorf("failed to parse operator tag %q as semantic version: %w", version, err)
	}

	return SupportsAdditionalPrinterColumnsConstraint.Check(semVer), nil
}
