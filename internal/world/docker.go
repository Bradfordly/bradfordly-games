package world

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const dockerAPIPrefix = "/v1.41"

// ErrNotExist is returned when a container is missing.
var ErrNotExist = errors.New("container not found")

// Docker talks to the local Docker Engine API. It never starts containers.
type Docker struct {
	client *http.Client
	base   string
}

// NewDocker connects to host (unix:///var/run/docker.sock or tcp://...).
func NewDocker(host string) (*Docker, error) {
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("docker host: %w", err)
	}
	d := &Docker{base: "http://docker" + dockerAPIPrefix}
	switch u.Scheme {
	case "unix":
		d.client = &http.Client{
			Timeout: 2 * time.Minute,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", u.Path)
				},
			},
		}
	case "tcp", "http":
		base := "http://" + u.Host + dockerAPIPrefix
		if u.Scheme == "http" && u.Host != "" {
			base = "http://" + u.Host + dockerAPIPrefix
		}
		d.base = base
		d.client = &http.Client{Timeout: 2 * time.Minute}
	default:
		return nil, fmt.Errorf("docker host scheme %q", u.Scheme)
	}
	return d, nil
}

func (d *Docker) EnsureStopped(ctx context.Context, spec ContainerSpec) error {
	have, err := d.Inspect(ctx, spec.Name)
	if err == nil {
		if have.Running {
			return fmt.Errorf("container %s is running; drain before replacing", spec.Name)
		}
		if !specNeedsRecreate(have, spec) {
			return nil
		}
		if err := d.Remove(ctx, spec.Name); err != nil {
			return err
		}
	} else if !errors.Is(err, ErrNotExist) {
		return err
	}
	if err := d.create(ctx, spec); err != nil {
		if isNoSuchImage(err) {
			if pullErr := d.pull(ctx, spec.Image); pullErr != nil {
				return pullErr
			}
			return d.create(ctx, spec)
		}
		return err
	}
	return nil
}

func (d *Docker) Remove(ctx context.Context, name string) error {
	resp, err := d.do(ctx, http.MethodDelete, "/containers/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode >= 300 {
		return dockerError(resp)
	}
	return nil
}

func (d *Docker) Inspect(ctx context.Context, name string) (Container, error) {
	resp, err := d.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(name)+"/json", nil)
	if err != nil {
		return Container{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Container{}, ErrNotExist
	}
	if resp.StatusCode >= 300 {
		return Container{}, dockerError(resp)
	}
	var raw inspectResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Container{}, err
	}
	ports := map[string][]string{}
	for port, binds := range raw.HostConfig.PortBindings {
		for _, b := range binds {
			if b.HostPort != "" {
				ports[port] = append(ports[port], b.HostPort)
			}
		}
	}
	name = strings.TrimPrefix(raw.Name, "/")
	return Container{
		Name:         name,
		Image:        raw.Config.Image,
		Env:          raw.Config.Env,
		Binds:        raw.HostConfig.Binds,
		Memory:       raw.HostConfig.Memory,
		NanoCPUs:     raw.HostConfig.NanoCpus,
		Running:      raw.State.Running,
		PortBindings: ports,
	}, nil
}

func (d *Docker) RunningCount(ctx context.Context) (int, error) {
	resp, err := d.do(ctx, http.MethodGet, "/containers/json?filters="+url.QueryEscape(`{"label":["bradfordly.world.id"]}`), nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, dockerError(resp)
	}
	var list []struct {
		State string `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return 0, err
	}
	n := 0
	for _, c := range list {
		if c.State == "running" {
			n++
		}
	}
	return n, nil
}

func (d *Docker) create(ctx context.Context, spec ContainerSpec) error {
	body := createRequest{
		Image:  spec.Image,
		Env:    spec.Env,
		Labels: spec.Labels,
		HostConfig: createHostConfig{
			Binds:        spec.Binds,
			Memory:       spec.Memory,
			NanoCpus:     spec.NanoCPUs,
			PortBindings: map[string][]portBinding{},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := d.do(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(spec.Name), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return dockerError(resp)
	}
	return nil
}

func (d *Docker) pull(ctx context.Context, image string) error {
	resp, err := d.do(ctx, http.MethodPost, "/images/create?fromImage="+url.QueryEscape(image), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return dockerError(resp)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (d *Docker) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, d.base+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return d.client.Do(req)
}

func isNoSuchImage(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such image") || strings.Contains(msg, "not found")
}

func dockerError(resp *http.Response) error {
	b, _ := io.ReadAll(resp.Body)
	var msg struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &msg) == nil && msg.Message != "" {
		return fmt.Errorf("docker: %s", msg.Message)
	}
	return fmt.Errorf("docker: HTTP %d: %s", resp.StatusCode, bytes.TrimSpace(b))
}

type createRequest struct {
	Image      string            `json:"Image"`
	Env        []string          `json:"Env"`
	Labels     map[string]string `json:"Labels"`
	HostConfig createHostConfig  `json:"HostConfig"`
}

type createHostConfig struct {
	Binds        []string                 `json:"Binds"`
	Memory       int64                    `json:"Memory"`
	NanoCpus     int64                    `json:"NanoCpus"`
	PortBindings map[string][]portBinding `json:"PortBindings"`
}

type portBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}

type inspectResponse struct {
	Name  string `json:"Name"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
	Config struct {
		Image string   `json:"Image"`
		Env   []string `json:"Env"`
	} `json:"Config"`
	HostConfig struct {
		Binds        []string                 `json:"Binds"`
		Memory       int64                    `json:"Memory"`
		NanoCpus     int64                    `json:"NanoCpus"`
		PortBindings map[string][]portBinding `json:"PortBindings"`
	} `json:"HostConfig"`
}
