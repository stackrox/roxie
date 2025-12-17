package deployer

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

type FeatureFlags = *FeatureFlagsStruct

type FeatureFlagsStruct struct {
	m map[string]bool
}

func NewFeatureFlags() FeatureFlags {
	return &FeatureFlagsStruct{
		m: make(map[string]bool),
	}
}

func (f FeatureFlags) ToEnvVars() []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0, len(f.m))
	for featureFlag, val := range f.m {
		envVars = append(envVars, corev1.EnvVar{
			Name:  featureFlag,
			Value: strconv.FormatBool(val),
		})
	}
	return envVars
}

func (f FeatureFlags) set(featureFlag string, val bool) error {
	if !strings.HasPrefix(featureFlag, "ROX_") {
		return fmt.Errorf("invalid feature flag name: %s", featureFlag)
	}
	f.m[featureFlag] = val
	return nil
}

func (f FeatureFlags) parseAndSetOne(setting string) error {
	featureFlag, valStr, found := strings.Cut(setting, "=")
	if found {
		val, err := strconv.ParseBool(valStr)
		if err != nil {
			return fmt.Errorf("invalid feature flag value: %s", valStr)
		}
		return f.set(featureFlag, val)
	}

	// Look at first character to determine boolean value.
	val := true
	switch featureFlag[0] {
	case '+':
		featureFlag = featureFlag[1:]
	case '-':
		featureFlag = featureFlag[1:]
		val = false
	}
	return f.set(featureFlag, val)
}

func (f FeatureFlags) ParseAndSetFromSlice(settingsSlice []string) error {
	for _, settingsString := range settingsSlice {
		settings := strings.Split(settingsString, ",")
		if err := f.parseAndSetFromString(settingsString); err != nil {
			return err
		}
		for _, setting := range settings {
			setting = strings.TrimSpace(setting)
			if err := f.parseAndSetOne(setting); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f FeatureFlags) parseAndSetFromString(settingsString string) error {
	settings := strings.Split(settingsString, ",")
	for _, setting := range settings {
		setting = strings.TrimSpace(setting)
		if err := f.parseAndSetOne(setting); err != nil {
			return err
		}
	}
	return nil
}
