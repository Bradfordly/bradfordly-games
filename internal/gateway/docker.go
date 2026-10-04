package gateway

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ContainerEngine starts and inspects world containers. The real client talks
// to a local Docker API or CLI. It must never listen on the Elastic IP.
type ContainerEngine interface {
	Start(ctx context.Context, container string) error
	Running(ctx context.Context, container string) (bool, error)
	ListRunning(ctx context.Context) ([]string, error)
}

// CLIEngine calls the docker CLI. DOCKER_HOST may point at a local socket
// or a localhost TCP proxy, not a public address.
type CLIEngine struct {
	Bin  string
	Host string
}

func (e CLIEngine) bin() string {
	if e.Bin == "" {
		return "docker"
	}
	return e.Bin
}

func (e CLIEngine) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, e.bin(), args...)
	if e.Host != "" {
		cmd.Env = append(cmd.Environ(), "DOCKER_HOST="+e.Host)
	}
	return cmd
}

func (e CLIEngine) Start(ctx context.Context, container string) error {
	if container == "" {
		return fmt.Errorf("missing container name")
	}
	cmd := e.command(ctx, "start", container)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker start %s: %w: %s", container, err, bytes.TrimSpace(out))
	}
	return nil
}

func (e CLIEngine) Running(ctx context.Context, container string) (bool, error) {
	if container == "" {
		return false, fmt.Errorf("missing container name")
	}
	cmd := e.command(ctx, "inspect", "-f", "{{.State.Running}}", container)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("docker inspect %s: %w: %s", container, err, bytes.TrimSpace(out))
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func (e CLIEngine) ListRunning(ctx context.Context) ([]string, error) {
	cmd := e.command(ctx, "ps", "--format", "{{.Names}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w: %s", err, bytes.TrimSpace(out))
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}
