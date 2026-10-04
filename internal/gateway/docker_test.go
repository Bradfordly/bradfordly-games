package gateway

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIEngineUsesLocalDocker(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "docker")
	logPath := filepath.Join(dir, "calls.log")
	body := "#!/bin/sh\necho \"$DOCKER_HOST $*\" >> \"" + logPath + "\"\n" +
		"if [ \"$1\" = inspect ]; then echo true; fi\n" +
		"if [ \"$1\" = ps ]; then echo survival; fi\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	e := CLIEngine{Bin: script, Host: "unix:///var/run/docker.sock"}
	if err := e.Start(context.Background(), "survival"); err != nil {
		t.Fatal(err)
	}
	running, err := e.Running(context.Background(), "survival")
	if err != nil || !running {
		t.Fatalf("running = %v %v", running, err)
	}
	names, err := e.ListRunning(context.Background())
	if err != nil || len(names) != 1 || names[0] != "survival" {
		t.Fatalf("list = %v %v", names, err)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(logged) == "" {
		t.Fatal("expected docker CLI calls")
	}
}

func TestCLIEngineMissingName(t *testing.T) {
	e := CLIEngine{Bin: "/bin/false"}
	if err := e.Start(context.Background(), ""); err == nil {
		t.Fatal("expected error")
	}
	if _, err := e.Running(context.Background(), ""); err == nil {
		t.Fatal("expected inspect error")
	}
}

func TestCLIEngineCommandFailure(t *testing.T) {
	e := CLIEngine{Bin: "/bin/false"}
	if err := e.Start(context.Background(), "survival"); err == nil {
		t.Fatal("expected start error")
	}
	if _, err := e.Running(context.Background(), "survival"); err == nil {
		t.Fatal("expected inspect error")
	}
	if _, err := e.ListRunning(context.Background()); err == nil {
		t.Fatal("expected ps error")
	}
}
