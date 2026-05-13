package stackroxversions

import (
	"fmt"
	"strings"

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
	// We also need to support versions such as 4.11.0-937-gf0da38f1a.
	version, _, _ = strings.Cut(version, "-")
	semVer, err := semver.NewVersion(version)
	if err != nil {
		return false, fmt.Errorf("failed to parse operator tag %q as semantic version: %w", version, err)
	}

	return SupportsAdditionalPrinterColumnsConstraint.Check(semVer), nil
}
