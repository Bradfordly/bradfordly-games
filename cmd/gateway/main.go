package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/gateway"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	adminAddr := flag.String("admin-addr", gateway.DefaultAdminAddr, "localhost HTTP admin listen address")
	playerPort := flag.String("player-port", "25565", "host TCP port for Minecraft Java (mc-router)")
	worldsPath := flag.String("worlds", "", "JSON world store path")
	dockerBin := flag.String("docker", "docker", "docker CLI used to start world containers")
	dockerHost := flag.String("docker-host", "", "Docker host URL; localhost socket or TCP only")
	mcRouter := flag.String("mc-router", os.Getenv("MC_ROUTER_BIN"), "itzg/mc-router binary; empty skips the player listener")
	routerDir := flag.String("router-dir", "", "directory for generated mc-router routes")
	flag.Parse()

	if err := validateListen(*adminAddr, *playerPort); err != nil {
		return err
	}

	worlds, err := gateway.LoadWorlds(*worldsPath)
	if err != nil {
		return err
	}

	engine := gateway.CLIEngine{Bin: *dockerBin, Host: *dockerHost}
	rt := gateway.NewRuntime(worlds, engine)

	ln, err := net.Listen("tcp", *adminAddr)
	if err != nil {
		return fmt.Errorf("admin listen: %w", err)
	}
	actualAdmin := ln.Addr().String()
	log.Printf("gateway admin listening on %s", actualAdmin)
	log.Printf("gateway adapters: %s, %s, %s", adapter.GameMinecraftJava, adapter.GameValheim, adapter.GamePalworld)

	srv := &http.Server{Handler: gateway.AdminMux(rt)}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	var routerCmd = (*os.Process)(nil)
	if *mcRouter != "" {
		dir := *routerDir
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "bradfordly-mc-router")
		}
		routesPath, allowPath, err := gateway.WriteRouterFiles(dir, worlds)
		if err != nil {
			return fmt.Errorf("mc-router config: %w", err)
		}
		hook, err := gateway.WebhookURL(actualAdmin)
		if err != nil {
			return err
		}
		asleep, starting := gateway.DefaultMOTDs(worlds)
		cmd := gateway.RouterCommand(*mcRouter, *playerPort, routesPath, allowPath, hook, asleep, starting, gateway.DefaultHoldWindow)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("start mc-router: %w", err)
		}
		routerCmd = cmd.Process
		log.Printf("mc-router listening on host TCP %s (docker start stays in this process)", *playerPort)
	} else {
		log.Printf("mc-router binary not set; player listener disabled")
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		if routerCmd != nil {
			_ = routerCmd.Signal(syscall.SIGTERM)
		}
		return err
	case <-sig:
		if routerCmd != nil {
			_ = routerCmd.Signal(syscall.SIGTERM)
		}
		return srv.Close()
	}
}

func validateListen(adminAddr, playerPort string) error {
	if gateway.AdminIsPlayerPort(adminAddr) {
		return fmt.Errorf("admin port must not be 25565")
	}
	if !gateway.AdminIsLocalhost(adminAddr) {
		return fmt.Errorf("admin address %q must bind localhost, not the Elastic IP", adminAddr)
	}
	if playerPort == "25565" || playerPort == "" {
		return nil
	}
	return nil
}
