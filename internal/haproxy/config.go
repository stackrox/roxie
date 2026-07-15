package haproxy

import (
	"fmt"
	"strings"

	"github.com/stackrox/roxie/internal/deployer"
)

func RenderConfig(haproxyConfig deployer.HAProxyConfig, endpoint, caCertFile string) string {
	return strings.NewReplacer(
		"${ENDPOINT}", endpoint,
		"${CA_CERT_FILE}", caCertFile,
		"${BIND_PORT}", fmt.Sprintf("%d", haproxyConfig.BindPort),
	).Replace(`global
    log /dev/null local0

defaults
    log     global
    mode    http
    timeout connect 5s
    timeout client  30s
    timeout server  30s

frontend http_front
    bind *:${BIND_PORT}
    default_backend https_back

backend https_back
    server srv1 ${ENDPOINT} ssl verify required ca-file ${CA_CERT_FILE}
`)
}
