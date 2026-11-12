//go:build e2e

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildRoxie(t *testing.T, version string) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "roxie-"+version)
	cmd := exec.Command("make", "build", "ROXIE_VERSION="+version, "BINARY="+binary)
	cmd.Dir = repoRoot(t)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "failed to build roxie with version %s", version)
	return binary
}

func repoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/stackrox/roxie")
	out, err := cmd.Output()
	require.NoError(t, err, "failed to locate repo root")
	return strings.TrimSpace(string(out))
}

func roxieVersion(t *testing.T, binary string) string {
	t.Helper()
	versionRe := regexp.MustCompile(`roxie version v?(\S+)`)
	out, err := exec.Command(binary, "version").Output()
	require.NoError(t, err, "failed to get version")
	m := versionRe.FindSubmatch(out)
	require.NotNil(t, m, "unexpected version output: %s", string(out))
	return string(m[1])
}

func TestUpdateFromOldVersion(t *testing.T) {
	old := "0.0.1"
	binary := buildRoxie(t, old)
	link := filepath.Join(t.TempDir(), "roxie-link")
	require.NoError(t, os.Symlink(binary, link))

	output := roxieVersion(t, link)
	assert.Equal(t, old, output, "binary should report the old version before update")

	cmd := exec.Command(link, "update")
	outBytes, err := cmd.CombinedOutput()
	out := string(outBytes)
	require.NoError(t, err, "update command failed: %s", out)
	assert.Contains(t, out, "Updated roxie:")

	ver := roxieVersion(t, binary)
	assert.NotEqual(t, old, ver, "binary version should have changed after update")
}

func TestUpdateForceReinstalls(t *testing.T) {
	binary := buildRoxie(t, "0.0.1")

	cmd := exec.Command(binary, "update", "--force")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "forced update failed: %s", string(out))
	assert.Contains(t, string(out), "Force flag set, skipping version comparison")
	assert.Contains(t, string(out), "Updated roxie:")
}

func TestUpdateRejectsDevBuild(t *testing.T) {
	binary := buildRoxie(t, "dev")

	cmd := exec.Command(binary, "update")
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "update should fail for dev builds")
	assert.Contains(t, string(out), "cannot self-update a development build")
}

func TestUpdateFromGitDescribeVersion(t *testing.T) {
	binary := buildRoxie(t, "0.0.1-3-gabcdef0")

	ver := roxieVersion(t, binary)
	assert.Equal(t, "0.0.1-3-gabcdef0", ver)

	cmd := exec.Command(binary, "update")
	outBytes, err := cmd.CombinedOutput()
	out := string(outBytes)
	require.NoError(t, err, "update command failed: %s", out)

	assert.Contains(t, out, "Detected non-release version")
	assert.Contains(t, out, "Updated roxie:")
}
