package containerutil

import (
	"github.com/moby/sys/mountinfo"
)

// IsRunningInContainer detects if the current process is running inside
// a Docker, Podman, or Kubernetes container by checking if the root
// filesystem is an overlay filesystem (standard for containers).
func IsRunningInContainer() bool {
	// Get root mount info using efficient filter
	rootMounts, err := mountinfo.GetMounts(mountinfo.SingleEntryFilter("/"))
	if err != nil || len(rootMounts) == 0 {
		return false
	}

	// Containers use overlay filesystem for root
	return rootMounts[0].FSType == "overlay"
}
