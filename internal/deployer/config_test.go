package deployer

import (
	"testing"

	"github.com/stackrox/roxie/internal/imagetag"
	"github.com/stretchr/testify/assert"
)

// A manifest written by old roxie baked operator.version / operator.konfluxImages
// equal to the roxie values; both are dropped so they re-resolve (e.g. a later
// --tag takes effect).
func TestDropBakedOperatorDefaults_ClearsBaked(t *testing.T) {
	cfg := Config{
		Roxie: RoxieConfig{Version: "4.10.0", KonfluxImages: new(true)},
		Operator: OperatorConfig{OperatorInstanceConfig: OperatorInstanceConfig{
			Version:       "4.10.0",
			KonfluxImages: new(true),
		}},
	}
	cfg.DropBakedOperatorDefaults()

	assert.Empty(t, cfg.Operator.Version)
	assert.Nil(t, cfg.Operator.KonfluxImages)

	// --tag updates roxie.version; the operator version follows.
	cfg.Roxie.Version = "4.11.0"
	assert.Equal(t, imagetag.MainTag("4.11.0"), cfg.CentralVersion())
	assert.Equal(t, imagetag.MainTag("4.11.0"), cfg.SecuredClusterVersion())
}

// Operator values that differ from the roxie values were set deliberately and
// must be preserved.
func TestDropBakedOperatorDefaults_PreservesDeliberate(t *testing.T) {
	cfg := Config{
		Roxie: RoxieConfig{Version: "4.11.0", KonfluxImages: new(false)},
		Operator: OperatorConfig{OperatorInstanceConfig: OperatorInstanceConfig{
			Version:       "4.8.0",
			KonfluxImages: new(true),
		}},
	}
	cfg.DropBakedOperatorDefaults()

	assert.Equal(t, imagetag.MainTag("4.8.0"), cfg.Operator.Version)
	if assert.NotNil(t, cfg.Operator.KonfluxImages) {
		assert.True(t, *cfg.Operator.KonfluxImages)
	}
}
