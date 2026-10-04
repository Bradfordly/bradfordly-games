package world

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Gateway is the panel's view of the edge gateway admin API.
type Gateway interface {
	Drain(ctx context.Context, worldID string) error
	Reload(ctx context.Context) error
}

// HTTPGateway pushes drain and reload to the gateway admin port.
type HTTPGateway struct {
	Base   string
	Client *http.Client
}

// NewHTTPGateway returns a client for the gateway admin server.
func NewHTTPGateway(base string) *HTTPGateway {
	return &HTTPGateway{
		Base:   strings.TrimRight(base, "/"),
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *HTTPGateway) Drain(ctx context.Context, worldID string) error {
	return g.post(ctx, "/worlds/"+url.PathEscape(worldID)+"/drain")
}

func (g *HTTPGateway) Reload(ctx context.Context) error {
	return g.post(ctx, "/reload")
}

func (g *HTTPGateway) post(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.Base+path, nil)
	if err != nil {
		return err
	}
	resp, err := g.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway %s: HTTP %d", path, resp.StatusCode)
	}
	return nil
}
