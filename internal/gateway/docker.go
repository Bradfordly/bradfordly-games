package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const dockerAPI = "/v1.43"

// DockerRuntime talks to the local Docker Engine API. It never calls AWS.
type DockerRuntime struct {
	client *http.Client
	base   string
	poll   time.Duration
}

// NewDockerRuntime dials the Docker unix socket (default /var/run/docker.sock).
func NewDockerRuntime(sock string) *DockerRuntime {
	if sock == "" {
		sock = "/var/run/docker.sock"
	}
	return &DockerRuntime{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", sock)
				},
			},
		},
		base: "http://docker",
		poll: 200 * time.Millisecond,
	}
}

// NewDockerHTTPRuntime is for tests against an httptest Docker stand-in.
func NewDockerHTTPRuntime(client *http.Client, base string) *DockerRuntime {
	return &DockerRuntime{client: client, base: base, poll: 5 * time.Millisecond}
}

func (d *DockerRuntime) Running(ctx context.Context, container string) (bool, error) {
	state, err := d.inspect(ctx, container)
	if err != nil {
		return false, err
	}
	return state.Running, nil
}

func (d *DockerRuntime) Stop(ctx context.Context, container string, timeout time.Duration) (bool, error) {
	if err := d.kill(ctx, container, "SIGTERM"); err != nil {
		if isNotRunning(err) {
			return false, nil
		}
		return false, err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := d.Running(ctx, container)
		if err != nil {
			if isNotFound(err) {
				return false, nil
			}
			return false, err
		}
		if !running {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(d.poll):
		}
	}
	if err := d.kill(ctx, container, "SIGKILL"); err != nil && !isNotRunning(err) && !isNotFound(err) {
		return false, err
	}
	return true, nil
}

type dockerState struct {
	Running bool `json:"Running"`
}

func (d *DockerRuntime) inspect(ctx context.Context, container string) (dockerState, error) {
	var wrap struct {
		State dockerState `json:"State"`
	}
	path := dockerAPI + "/containers/" + url.PathEscape(container) + "/json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+path, nil)
	if err != nil {
		return dockerState{}, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return dockerState{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return dockerState{}, &dockerError{status: resp.StatusCode, msg: string(body)}
	}
	if resp.StatusCode >= 300 {
		return dockerState{}, fmt.Errorf("docker inspect %s: %s", container, bytesPreview(body))
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return dockerState{}, err
	}
	return wrap.State, nil
}

func (d *DockerRuntime) kill(ctx context.Context, container, signal string) error {
	path := dockerAPI + "/containers/" + url.PathEscape(container) + "/kill?signal=" + url.QueryEscape(signal)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusConflict {
		return &dockerError{status: resp.StatusCode, msg: string(body)}
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("docker kill %s %s: %s", signal, container, bytesPreview(body))
	}
	return nil
}

type dockerError struct {
	status int
	msg    string
}

func (e *dockerError) Error() string {
	return fmt.Sprintf("docker: %d %s", e.status, e.msg)
}

func isNotRunning(err error) bool {
	de, ok := err.(*dockerError)
	return ok && de.status == http.StatusConflict
}

func isNotFound(err error) bool {
	de, ok := err.(*dockerError)
	return ok && de.status == http.StatusNotFound
}

func bytesPreview(b []byte) string {
	if len(b) > 200 {
		return string(b[:200])
	}
	return string(b)
}
