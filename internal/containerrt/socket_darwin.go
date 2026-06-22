package containerrt

import (
	"os"
	"path/filepath"
)

func platformSocketCandidates() []string {
	var candidates []string

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(home, ".local", "share", "containers", "podman", "machine", "podman.sock"),
		)
	}

	return candidates
}
