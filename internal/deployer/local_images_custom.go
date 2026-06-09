package deployer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/stackrox/roxie/internal/logger"
)

type customImagePreLoader struct {
	log     *logger.Logger
	command string
}

func NewCustomImagePreloader(_ context.Context, log *logger.Logger, command string) ImagePreLoader {
	return &customImagePreLoader{
		log:     log,
		command: command,
	}
}

func (c *customImagePreLoader) GetImages(_ context.Context) ([]string, error) {
	return []string{}, ErrLocalImageRetrievalNotSupported
}

func (c *customImagePreLoader) SendImage(ctx context.Context, image string) error {
	env := os.Environ()
	env = append(env, fmt.Sprintf("IMAGE=%s", image))
	c.log.Dimf("Invoking %q...", c.command)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", c.command)
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		c.log.Warningf("Image preloading failed: %v", err)
		for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
			c.log.Dimf("| %s", line)
		}
		return fmt.Errorf("sending image failed: %w", err)
	}
	return nil
}

func (c *customImagePreLoader) Name() string {
	return "custom image preloader"
}
