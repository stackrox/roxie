package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMarshalYAMLSkipsCACertFile(t *testing.T) {
	roxieEnv := RoxieEnvironment{
		RoxUsername:   "davinci",
		RoxCaCertFile: "/some/file.pem",
	}

	roxieEnvBytes, err := yaml.Marshal(roxieEnv)
	require.NoError(t, err, "YAML marshaling failed")

	var roxieEnvUnmarshaled map[string]string
	err = yaml.Unmarshal(roxieEnvBytes, &roxieEnvUnmarshaled)
	require.NoError(t, err, "YAML unmarshaling failed")

	assert.NotEmpty(t, roxieEnvUnmarshaled["ROX_USERNAME"], "ROX_USERNAME missing")
	_, ok := roxieEnvUnmarshaled["ROX_CA_CERT_FILE"]
	assert.False(t, ok, "ROX_CA_CERT_FILE present")
}
