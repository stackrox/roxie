package imagetag

import "strings"

// MainTag is a StackRox image tag as provided by the user (e.g. "4.11.x-dirty",
// "4.11.1", "4.11.0-937-gf0da38f1a"). All version fields in the config store this form.
type MainTag string

// OperatorTag is a semver-compatible tag derived from a MainTag, suitable for
// operator bundle images, Konflux image references, and version comparisons.
type OperatorTag string

// ToOperatorTag converts a MainTag to an OperatorTag by removing "-dirty" and
// replacing ".x" with ".0". The conversion is idempotent.
func (t MainTag) ToOperatorTag() OperatorTag {
	if t == "" {
		return ""
	}
	s := strings.ReplaceAll(string(t), "-dirty", "")
	s = strings.ReplaceAll(s, ".x", ".0")
	return OperatorTag(s)
}

func (t MainTag) String() string     { return string(t) }
func (t OperatorTag) String() string { return string(t) }
