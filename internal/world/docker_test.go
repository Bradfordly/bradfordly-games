package world

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDockerRejectsBadScheme(t *testing.T) {
	if _, err := NewDocker("ftp://x"); err == nil {
		t.Fatal("expected scheme error")
	}
}

func TestNewDockerUnixAndEmptyHost(t *testing.T) {
	d, err := NewDocker("")
	if err != nil {
		t.Fatal(err)
	}
	if d.base != "http://docker"+dockerAPIPrefix {
		t.Fatalf("base = %q", d.base)
	}
	d, err = NewDocker("unix:///var/run/docker.sock")
	if err != nil {
		t.Fatal(err)
	}
	if d.client == nil {
		t.Fatal("unix client")
	}
}

func TestDockerEnsureStoppedCreateInspectRemove(t *testing.T) {
	eng := newFakeEngine()
	srv := httptest.NewServer(eng.mux())
	t.Cleanup(srv.Close)
	d := &Docker{client: srv.Client(), base: srv.URL + dockerAPIPrefix}

	spec := mustSpec(t)
	if err := d.EnsureStopped(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	c, err := d.Inspect(context.Background(), spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	if c.Running || c.Image != spec.Image || c.Memory != spec.Memory || c.NanoCPUs != spec.NanoCPUs {
		t.Fatalf("inspect = %#v", c)
	}
	if publishedRCON(c) {
		t.Fatal("RCON published")
	}
	if !stringSetsEqual(c.Binds, spec.Binds) {
		t.Fatalf("binds = %#v", c.Binds)
	}
	if envMap(c.Env)["EULA"] != "TRUE" {
		t.Fatalf("env = %#v", c.Env)
	}

	// idempotent when unchanged
	if err := d.EnsureStopped(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if eng.creates != 1 {
		t.Fatalf("creates = %d, want 1", eng.creates)
	}

	spec.Env = append(spec.Env, "DIFFICULTY=hard")
	if err := d.EnsureStopped(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	if eng.creates != 2 {
		t.Fatalf("recreate creates = %d", eng.creates)
	}
	c, err = d.Inspect(context.Background(), spec.Name)
	if err != nil {
		t.Fatal(err)
	}
	if envMap(c.Env)["DIFFICULTY"] != "hard" {
		t.Fatalf("env after update = %#v", c.Env)
	}

	if n, err := d.RunningCount(context.Background()); err != nil || n != 0 {
		t.Fatalf("running = %d err=%v", n, err)
	}
	eng.setRunning(spec.Name, true)
	if n, err := d.RunningCount(context.Background()); err != nil || n != 1 {
		t.Fatalf("running after start = %d err=%v", n, err)
	}
	eng.setRunning(spec.Name, false)
	if err := d.Remove(context.Background(), spec.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Inspect(context.Background(), spec.Name); err != ErrNotExist {
		t.Fatalf("inspect after remove = %v", err)
	}
	if err := d.Remove(context.Background(), spec.Name); err != nil {
		t.Fatal("remove missing should be ok")
	}
}

func TestDockerPullsMissingImage(t *testing.T) {
	eng := newFakeEngine()
	eng.missingImage = true
	srv := httptest.NewServer(eng.mux())
	t.Cleanup(srv.Close)
	d := &Docker{client: srv.Client(), base: srv.URL + dockerAPIPrefix}
	if err := d.EnsureStopped(context.Background(), mustSpec(t)); err != nil {
		t.Fatal(err)
	}
	if !eng.pulled {
		t.Fatal("expected image pull")
	}
}

func TestDockerPullFailure(t *testing.T) {
	eng := newFakeEngine()
	eng.missingImage = true
	eng.failPull = true
	srv := httptest.NewServer(eng.mux())
	t.Cleanup(srv.Close)
	d := &Docker{client: srv.Client(), base: srv.URL + dockerAPIPrefix}
	if err := d.EnsureStopped(context.Background(), mustSpec(t)); err == nil {
		t.Fatal("expected pull error")
	}
}

func TestDockerRefusesReplaceWhileRunning(t *testing.T) {
	eng := newFakeEngine()
	srv := httptest.NewServer(eng.mux())
	t.Cleanup(srv.Close)
	d := &Docker{client: srv.Client(), base: srv.URL + dockerAPIPrefix}
	spec := mustSpec(t)
	if err := d.EnsureStopped(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	eng.setRunning(spec.Name, true)
	spec.Env = append(spec.Env, "DIFFICULTY=hard")
	if err := d.EnsureStopped(context.Background(), spec); err == nil {
		t.Fatal("expected running error")
	}
}

func TestDockerHTTPErrors(t *testing.T) {
	eng := newFakeEngine()
	eng.failInspect = true
	srv := httptest.NewServer(eng.mux())
	t.Cleanup(srv.Close)
	d := &Docker{client: srv.Client(), base: srv.URL + dockerAPIPrefix}
	if _, err := d.Inspect(context.Background(), "x"); err == nil {
		t.Fatal("expected inspect error")
	}
}

func TestDockerErrorRawBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not-json", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	d := &Docker{client: srv.Client(), base: srv.URL}
	if _, err := d.Inspect(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDiskFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "worlds", "id")
	var files diskFiles
	if err := files.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := files.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

func TestNewDockerTCP(t *testing.T) {
	d, err := NewDocker("tcp://127.0.0.1:2375")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.base, "127.0.0.1:2375") {
		t.Fatalf("base = %q", d.base)
	}
	if _, err := NewDocker("http://127.0.0.1:2375"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewDocker("://bad"); err == nil {
		t.Fatal("expected parse error")
	}
}

func mustSpec(t *testing.T) ContainerSpec {
	t.Helper()
	w := sampleWorld("id", "Survival")
	spec, err := containerSpec(w)
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

type fakeEngine struct {
	containers   map[string]inspectResponse
	creates      int
	pulled       bool
	missingImage bool
	failInspect  bool
	failPull     bool
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{containers: map[string]inspectResponse{}}
}

func (e *fakeEngine) setRunning(name string, running bool) {
	c := e.containers[name]
	c.State.Running = running
	e.containers[name] = c
}

func (e *fakeEngine) mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(dockerAPIPrefix+"/containers/create", e.create)
	mux.HandleFunc(dockerAPIPrefix+"/containers/{name}/json", e.inspect)
	mux.HandleFunc(dockerAPIPrefix+"/containers/{name}", e.delete)
	mux.HandleFunc(dockerAPIPrefix+"/containers/json", e.list)
	mux.HandleFunc(dockerAPIPrefix+"/images/create", e.pull)
	return mux
}

func (e *fakeEngine) create(w http.ResponseWriter, r *http.Request) {
	if e.missingImage && !e.pulled {
		http.Error(w, `{"message":"No such image"}`, http.StatusNotFound)
		return
	}
	var body createRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name := r.URL.Query().Get("name")
	e.creates++
	e.containers[name] = inspectResponse{
		Name: "/" + name,
		Config: struct {
			Image string   `json:"Image"`
			Env   []string `json:"Env"`
		}{Image: body.Image, Env: body.Env},
		HostConfig: struct {
			Binds        []string                 `json:"Binds"`
			Memory       int64                    `json:"Memory"`
			NanoCpus     int64                    `json:"NanoCpus"`
			PortBindings map[string][]portBinding `json:"PortBindings"`
		}{
			Binds:        body.HostConfig.Binds,
			Memory:       body.HostConfig.Memory,
			NanoCpus:     body.HostConfig.NanoCpus,
			PortBindings: body.HostConfig.PortBindings,
		},
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"Id": name})
}

func (e *fakeEngine) inspect(w http.ResponseWriter, r *http.Request) {
	if e.failInspect {
		http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
		return
	}
	name := r.PathValue("name")
	c, ok := e.containers[name]
	if !ok {
		http.Error(w, `{"message":"No such container"}`, http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(c)
}

func (e *fakeEngine) delete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := e.containers[name]; !ok {
		http.Error(w, `{"message":"No such container"}`, http.StatusNotFound)
		return
	}
	delete(e.containers, name)
	w.WriteHeader(http.StatusNoContent)
}

func (e *fakeEngine) list(w http.ResponseWriter, _ *http.Request) {
	var out []map[string]string
	for _, c := range e.containers {
		state := "created"
		if c.State.Running {
			state = "running"
		}
		out = append(out, map[string]string{"State": state})
	}
	if out == nil {
		out = []map[string]string{}
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (e *fakeEngine) pull(w http.ResponseWriter, _ *http.Request) {
	if e.failPull {
		http.Error(w, `{"message":"pull failed"}`, http.StatusInternalServerError)
		return
	}
	e.pulled = true
	e.missingImage = false
	_, _ = io.WriteString(w, `{"status":"ok"}`)
}
