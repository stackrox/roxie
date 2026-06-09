package containerrt

import (
	"fmt"
	"os"
	"path/filepath"
)

func platformSocketCandidates() []string {
	var candidates []string

	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append(candidates, filepath.Join(xdg, "podman", "podman.sock"))
	}
	candidates = append(candidates,
		fmt.Sprintf("/run/user/%d/podman/podman.sock", os.Getuid()),
		"/run/podman/podman.sock",
		"/run/k3s/containerd/containerd.sock",
	)

	return candidates
}
