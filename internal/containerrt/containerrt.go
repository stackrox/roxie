package containerrt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/stackrox/roxie/internal/logger"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

func ListLocalImages(ctx context.Context, host string) ([]string, error) {
	cli, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating container runtime client: %w", err)
	}
	defer cli.Close()

	images, err := cli.ImageList(ctx, image.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing images: %w", err)
	}

	var tags []string
	for _, img := range images {
		for _, tag := range img.RepoTags {
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags, nil
}

// ExecInContainer runs a command inside a container and returns its stdout.
func ExecInContainer(ctx context.Context, host, containerName string, cmd []string) ([]byte, error) {
	cli, err := client.NewClientWithOpts(client.WithHost(host), client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating container runtime client: %w", err)
	}
	defer cli.Close()

	execResp, err := cli.ContainerExecCreate(ctx, containerName, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("creating exec in container %s: %w", containerName, err)
	}

	attach, err := cli.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, fmt.Errorf("attaching to exec in container %s: %w", containerName, err)
	}
	defer attach.Close()

	var stdout, stderr bytes.Buffer
	if _, err := stdcopy.StdCopy(&stdout, &stderr, attach.Reader); err != nil {
		return nil, fmt.Errorf("reading exec output from container %s: %w", containerName, err)
	}

	inspect, err := cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return nil, fmt.Errorf("inspecting exec in container %s: %w", containerName, err)
	}
	if inspect.ExitCode != 0 {
		return nil, fmt.Errorf("command %v in container %s exited with code %d: %s", cmd, containerName, inspect.ExitCode, stderr.String())
	}

	return stdout.Bytes(), nil
}

// ParseCrictlImages extracts image tags from `crictl images -o json` output.
func ParseCrictlImages(data []byte) ([]string, error) {
	var result struct {
		Images []struct {
			RepoTags []string `json:"repoTags"`
		} `json:"images"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parsing crictl output: %w", err)
	}
	var tags []string
	for _, img := range result.Images {
		for _, tag := range img.RepoTags {
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}
	return tags, nil
}

// ResolveSocket returns the container runtime socket URI by checking DOCKER_HOST,
// then probing well-known paths for Docker, Podman. Returns "" if none found.
func ResolveSocket(log *logger.Logger) string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		log.Dimf("Using container runtime socket from DOCKER_HOST: %s", host)
		return host
	}

	for _, candidate := range socketCandidates() {
		if _, err := os.Stat(candidate); err == nil {
			socket := "unix://" + candidate
			log.Dimf("Detected container runtime socket: %s", socket)
			return socket
		}
	}
	log.Dimf("No container runtime socket found")
	return ""
}

func socketCandidates() []string {
	candidates := []string{
		"/var/run/docker.sock",
	}
	candidates = append(candidates, platformSocketCandidates()...)
	return candidates
}
