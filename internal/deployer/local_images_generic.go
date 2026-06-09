package deployer

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"context"

	"github.com/stackrox/roxie/internal/logger"
)

type genericImageSender struct {
	log  *logger.Logger
	args []string
}

func newGenericImageSender(log *logger.Logger, args ...string) genericImageSender {
	return genericImageSender{
		log:  log,
		args: args,
	}
}

func (g *genericImageSender) SendImage(ctx context.Context, imageTag string) error {
	if len(g.args) == 0 {
		return errors.New("genericImageSender: no command configured")
	}

	args := make([]string, len(g.args))
	for i, arg := range g.args {
		if arg == "<image>" {
			args[i] = imageTag
		} else {
			args[i] = g.args[i]
		}
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		argsJoined := strings.Join(args, " ")
		g.log.Errorf("Executing %s failed:", argsJoined)
		for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
			g.log.Errorf("| %s", line)
		}
		return fmt.Errorf("executing '%s': %w", argsJoined, err)
	}
	return nil
}
