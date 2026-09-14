package deployer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	log "github.com/stackrox/roxie/internal/logger"
)

type customImagePreLoader struct {
	command string
}

func NewCustomImagePreloader(command string) ImagePreLoader {
	return &customImagePreLoader{
		command: command,
	}
}

func (c *customImagePreLoader) GetImages(_ context.Context) ([]string, error) {
	return []string{}, ErrLocalImageRetrievalNotSupported
}

func (c *customImagePreLoader) SendImage(ctx context.Context, image string) error {
	env := os.Environ()
	env = append(env, fmt.Sprintf("IMAGE=%s", image))
	log.Dimf("Invoking %q...", c.command)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", c.command)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Warningf("Image preloading failed: %v", err)
		for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
			log.Dimf("| %s", line)
		}
		return fmt.Errorf("sending image failed: %w", err)
	}
	return nil
}

func (c *customImagePreLoader) Name() string {
	return "custom image preloader"
}
