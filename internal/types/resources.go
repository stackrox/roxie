package types

import (
	"fmt"
	"slices"
	"strings"
)

type ResourceProfile int

const (
	ResourceProfileAcsDefaults ResourceProfile = iota
	ResourceProfileAuto
	ResourceProfileTiny
	ResourceProfileSmall
	ResourceProfileMedium
	ResourceProfileCI
)

var (
	resourceProfileNames = map[ResourceProfile]string{
		ResourceProfileAcsDefaults: "acs-defaults",
		ResourceProfileAuto:        "auto",
		ResourceProfileTiny:        "tiny",
		ResourceProfileSmall:       "small",
		ResourceProfileMedium:      "medium",
		ResourceProfileCI:          "ci",
	}

	resourceProfileValues = func() map[string]ResourceProfile {
		m := make(map[string]ResourceProfile, len(resourceProfileNames))
		for k, v := range resourceProfileNames {
			m[v] = k
		}
		return m
	}()
)

func (r ResourceProfile) String() string {
	if name, ok := resourceProfileNames[r]; ok {
		return name
	}
	return fmt.Sprintf("ResourceProfile(%d)", int(r))
}

func (r ResourceProfile) MarshalYAML() (interface{}, error) {
	if name, ok := resourceProfileNames[r]; ok {
		return name, nil
	}
	return nil, fmt.Errorf("unknown resource profile: %d", int(r))
}

func (r *ResourceProfile) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	profile, ok := resourceProfileValues[s]
	if !ok {
		return fmt.Errorf("unknown resource profile: %q", s)
	}
	*r = profile
	return nil
}

func ResourceProfiles() []string {
	resourceProfiles := make([]string, 0, len(resourceProfileNames))
	for _, name := range resourceProfileNames {
		resourceProfiles = append(resourceProfiles, name)
	}
	return resourceProfiles
}

func ResourceProfilesJoined() string {
	profiles := ResourceProfiles()
	slices.Sort(profiles)
	return strings.Join(profiles, ", ")
}
