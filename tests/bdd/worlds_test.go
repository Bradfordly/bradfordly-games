package bdd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/world"
	"github.com/cucumber/godog"
)

type panelWorld struct {
	t          *testing.T
	dataDir    string
	store      *world.Store
	runtime    *world.FakeRuntime
	gateway    *world.FakeGateway
	handler    http.Handler
	lastStatus int
	lastBody   []byte
	created    world.World
	savePath   string
}

func (p *panelWorld) reset() {
	if p.store != nil {
		_ = p.store.Close()
	}
	p.dataDir = p.t.TempDir()
	store, err := world.OpenStore(filepath.Join(p.dataDir, "panel.db"))
	if err != nil {
		p.t.Fatal(err)
	}
	p.store = store
	p.runtime = world.NewFakeRuntime()
	p.gateway = &world.FakeGateway{}
	svc := world.NewService(store, p.runtime, nil, p.gateway, p.dataDir, world.DefaultDomain)
	p.handler = world.Handler{Service: svc}.Mux()
	p.lastStatus = 0
	p.lastBody = nil
	p.created = world.World{}
	p.savePath = ""
}

func (p *panelWorld) aLocalPanel() error {
	p.reset()
	return nil
}

func (p *panelWorld) createNamed(name, game string) error {
	body, _ := json.Marshal(map[string]string{"name": name, "game": game})
	return p.do(http.MethodPost, "/api/worlds", body)
}

func (p *panelWorld) createsMinecraft(name string) error {
	if err := p.createNamed(name, "minecraft-java"); err != nil {
		return err
	}
	if err := json.Unmarshal(p.lastBody, &p.created); err != nil {
		return err
	}
	p.savePath = p.created.Volume
	return nil
}

func (p *panelWorld) createsValheim(name string) error {
	return p.createNamed(name, "valheim")
}

func (p *panelWorld) existingWorld(name string) error {
	return p.createsMinecraft(name)
}

func (p *panelWorld) patchesSettings() error {
	body := []byte(`{"idle_timeout":"30m","env":{"DIFFICULTY":"hard"}}`)
	if err := p.do(http.MethodPatch, "/api/worlds/"+p.created.ID, body); err != nil {
		return err
	}
	return json.Unmarshal(p.lastBody, &p.created)
}

func (p *panelWorld) deletesWorld() error {
	return p.do(http.MethodDelete, "/api/worlds/"+p.created.ID, nil)
}

func (p *panelWorld) deletesWorldDestroy() error {
	return p.do(http.MethodDelete, "/api/worlds/"+p.created.ID+"?destroy=true", nil)
}

func (p *panelWorld) sqliteRow(name string) error {
	got, err := p.store.Get(p.created.ID)
	if err != nil {
		return err
	}
	if got.Name != name {
		return failf("row name %q want %q", got.Name, name)
	}
	return nil
}

func (p *panelWorld) saveExists() error {
	st, err := os.Stat(p.created.Volume)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return failf("%s is not a directory", p.created.Volume)
	}
	return nil
}

func (p *panelWorld) stoppedContainer() error {
	c, err := p.runtime.Inspect(nil, p.created.Backend.Container)
	if err != nil {
		return err
	}
	if c.Running || c.Image != "itzg/minecraft-server" || worldEnv(c)["EULA"] != "TRUE" {
		return failf("container %#v", c)
	}
	return nil
}

func (p *panelWorld) bindMount() error {
	c, err := p.runtime.Inspect(nil, p.created.Backend.Container)
	if err != nil {
		return err
	}
	want := p.created.Volume + ":/data"
	for _, b := range c.Binds {
		if b == want {
			return nil
		}
	}
	return failf("binds %#v want %s", c.Binds, want)
}

func (p *panelWorld) noRCON() error {
	c, err := p.runtime.Inspect(nil, p.created.Backend.Container)
	if err != nil {
		return err
	}
	if len(c.PortBindings) != 0 {
		return failf("published ports %#v", c.PortBindings)
	}
	return nil
}

func (p *panelWorld) allocationHost() error {
	if !strings.HasSuffix(p.created.Allocation.Host, "."+world.DefaultDomain) {
		return failf("host %q", p.created.Allocation.Host)
	}
	return nil
}

func (p *panelWorld) requestRejected() error {
	if p.lastStatus == http.StatusBadRequest {
		return nil
	}
	return failf("status %d body %s", p.lastStatus, p.lastBody)
}

func (p *panelWorld) sqlitePatched() error {
	got, err := p.store.Get(p.created.ID)
	if err != nil {
		return err
	}
	if got.IdleTimeout != "30m0s" || got.Env["DIFFICULTY"] != "hard" {
		return failf("row %#v", got)
	}
	return nil
}

func (p *panelWorld) containerHasDifficulty() error {
	c, err := p.runtime.Inspect(nil, p.created.Backend.Container)
	if err != nil {
		return err
	}
	if worldEnv(c)["DIFFICULTY"] != "hard" {
		return failf("env %#v", c.Env)
	}
	return nil
}

func (p *panelWorld) sameSavePath() error {
	if p.created.Volume != p.savePath {
		return failf("volume %q -> %q", p.savePath, p.created.Volume)
	}
	return p.saveExists()
}

func (p *panelWorld) gatewayDrained() error {
	if len(p.gateway.Drains) == 1 && p.gateway.Drains[0] == p.created.ID {
		return nil
	}
	return failf("drains %#v", p.gateway.Drains)
}

func (p *panelWorld) containerGone() error {
	_, err := p.runtime.Inspect(nil, p.created.Backend.Container)
	if err == world.ErrNotExist {
		return nil
	}
	return failf("container still present: %v", err)
}

func (p *panelWorld) saveStillExists() error {
	return p.saveExists()
}

func (p *panelWorld) sqliteGone() error {
	_, err := p.store.Get(p.created.ID)
	if err == world.ErrNotFound {
		return nil
	}
	return failf("row still present: %v", err)
}

func (p *panelWorld) saveGone() error {
	if _, err := os.Stat(p.created.Volume); os.IsNotExist(err) {
		return nil
	}
	return failf("save still exists")
}

func (p *panelWorld) do(method, path string, body []byte) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	rec := httptest.NewRecorder()
	p.handler.ServeHTTP(rec, req)
	p.lastStatus = rec.Code
	p.lastBody = rec.Body.Bytes()
	return nil
}

func worldEnv(c world.Container) map[string]string {
	out := map[string]string{}
	for _, item := range c.Env {
		k, v, ok := strings.Cut(item, "=")
		if ok {
			out[k] = v
		}
	}
	return out
}

func failf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

func TestFeatures(t *testing.T) {
	p := &panelWorld{t: t}
	suite := godog.TestSuite{
		Name: "worlds",
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			ctx.Step(`^a local panel with a data volume and Docker API$`, p.aLocalPanel)
			ctx.Step(`^an operator creates a minecraft-java world named "([^"]*)"$`, p.createsMinecraft)
			ctx.Step(`^an operator creates a valheim world named "([^"]*)"$`, p.createsValheim)
			ctx.Step(`^an existing world named "([^"]*)"$`, p.existingWorld)
			ctx.Step(`^an operator patches idle_timeout to "30m" and env DIFFICULTY=hard$`, p.patchesSettings)
			ctx.Step(`^an operator deletes the world$`, p.deletesWorld)
			ctx.Step(`^an operator deletes the world and asks to destroy the save$`, p.deletesWorldDestroy)
			ctx.Step(`^a SQLite world row exists for "([^"]*)"$`, p.sqliteRow)
			ctx.Step(`^a save directory exists on the data volume$`, p.saveExists)
			ctx.Step(`^a stopped itzg/minecraft-server container exists with EULA=TRUE$`, p.stoppedContainer)
			ctx.Step(`^the container bind-mounts the save directory on /data$`, p.bindMount)
			ctx.Step(`^the container does not publish RCON$`, p.noRCON)
			ctx.Step(`^allocation\.host is a name under games\.bradfordly\.com$`, p.allocationHost)
			ctx.Step(`^the request is rejected$`, p.requestRejected)
			ctx.Step(`^the SQLite row has idle_timeout "30m0s" and DIFFICULTY hard$`, p.sqlitePatched)
			ctx.Step(`^the container env includes DIFFICULTY=hard$`, p.containerHasDifficulty)
			ctx.Step(`^the save directory is the same path$`, p.sameSavePath)
			ctx.Step(`^the gateway was asked to drain$`, p.gatewayDrained)
			ctx.Step(`^the container is gone$`, p.containerGone)
			ctx.Step(`^the save directory still exists$`, p.saveStillExists)
			ctx.Step(`^the SQLite row is gone$`, p.sqliteGone)
			ctx.Step(`^the save directory is gone$`, p.saveGone)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("bdd scenarios failed")
	}
}
