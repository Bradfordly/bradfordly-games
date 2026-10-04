package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/gateway"
)

func newGateway(worldsFile, dockerSock string) (*gateway.Supervisor, http.Handler, error) {
	plays := adapter.NewPlayCounts()
	adapters := adapter.NewRegistryWithPlayCounts(plays)
	log.Printf("gateway adapters: %s, %s, %s", adapter.GameMinecraftJava, adapter.GameValheim, adapter.GamePalworld)
	rt := gateway.NewDockerRuntime(dockerSock)
	sup := gateway.NewSupervisor(adapters, rt, plays)
	worlds, err := gateway.LoadWorldsFile(worldsFile)
	if err != nil {
		return nil, nil, err
	}
	for _, w := range worlds {
		sup.AddWorld(w)
	}
	return sup, adminMux(sup), nil
}

func main() {
	addr := flag.String("admin-addr", ":8080", "admin HTTP listen address")
	worldsFile := flag.String("worlds-file", "", "JSON file of world specs")
	dockerSock := flag.String("docker-sock", "/var/run/docker.sock", "Docker Engine unix socket")
	flag.Parse()

	sup, handler, err := newGateway(*worldsFile, *dockerSock)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go sup.Run(ctx)

	srv := &http.Server{Addr: *addr, Handler: handler}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	log.Printf("gateway admin listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
