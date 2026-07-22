package haproxy

import (
	"strings"
	"testing"

	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stretchr/testify/assert"
)

func TestRenderConfig(t *testing.T) {
	cfg := deployer.HAProxyConfig{BindPort: 8080}
	result := RenderConfig(cfg, "central.acs:443", "/tmp/ca.pem")

	assert.Contains(t, result, "bind *:8080")
	assert.Contains(t, result, "server srv1 central.acs:443 ssl verify required ca-file /tmp/ca.pem")
}

func TestRenderConfig_CustomPort(t *testing.T) {
	cfg := deployer.HAProxyConfig{BindPort: 9090}
	result := RenderConfig(cfg, "central.acs:443", "/tmp/ca.pem")

	assert.Contains(t, result, "bind *:9090")
	assert.NotContains(t, result, "8080")
}

func TestRenderConfig_NoUnresolvedPlaceholders(t *testing.T) {
	cfg := deployer.HAProxyConfig{BindPort: 8080}
	result := RenderConfig(cfg, "central.acs:443", "/tmp/ca.pem")

	assert.False(t, strings.Contains(result, "${"), "config should not contain unresolved placeholders")
}
