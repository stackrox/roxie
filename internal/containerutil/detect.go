package containerutil

import (
	"fmt"
	"os"
	"strconv"
)

// IsRunningInRoxieContainer checks if we are running inside the released roxie container.
// This knowledge allows us to adjust behavior accordingly, allowing for some UX improvements.
func IsRunningInRoxieContainer() bool {
	strVal, exists := os.LookupEnv("RUNNING_IN_ROXIE_CONTAINER")
	if !exists || strVal == "" {
		return false
	}
	val, err := strconv.ParseBool(strVal)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Invalid value for RUNNING_IN_ROXIE_CONTAINER: %s. Expected a boolean value (true/false). Defaulting to false.\n", strVal)
		return false
	}
	return val
}
