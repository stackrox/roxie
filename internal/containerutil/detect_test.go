package containerutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRunningInRoxieContainer(t *testing.T) {
	t.Setenv("RUNNING_IN_ROXIE_CONTAINER", "true")
	assert.True(t, IsRunningInRoxieContainer(), "Expected to detect running in roxie container when environment variable is set")

	t.Setenv("RUNNING_IN_ROXIE_CONTAINER", "")
	assert.False(t, IsRunningInRoxieContainer(), "Expected to not detect running in roxie container when environment variable is unset")

	t.Setenv("RUNNING_IN_ROXIE_CONTAINER", "garbage")
	assert.False(t, IsRunningInRoxieContainer(), "Expected to not detect running in roxie container when environment variable has invalid value")
}
