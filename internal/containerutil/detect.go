package containerutil

import (
	"os"
)

// IsRunningInRoxieContainer checks if we are running inside the released roxie container.
// This knowledge allows us to adjust behavior accordingly, allowing for some UX improvements.
func IsRunningInRoxieContainer() bool {
	_, exists := os.LookupEnv("RUNNING_IN_ROXIE_CONTAINER")
	return exists
}
