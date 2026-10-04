package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bradfordly/bradfordly-games/internal/version"
	"github.com/bradfordly/bradfordly-games/internal/world"
)

func main() {
	addr := flag.String("addr", ":8000", "panel HTTP listen address")
	dataDir := flag.String("data-dir", "/data", "data volume root for SQLite and world saves")
	sqlitePath := flag.String("sqlite", "", "SQLite file (default <data-dir>/panel.db)")
	gatewayURL := flag.String("gateway-url", "http://127.0.0.1:8080", "gateway admin base URL")
	domain := flag.String("allocation-domain", world.DefaultDomain, "suffix for allocation.host")
	dockerHost := flag.String("docker-host", "", "Docker host (default unix:///var/run/docker.sock)")
	flag.Parse()

	log.Printf("panel %s", version.String)

	dbPath := *sqlitePath
	if dbPath == "" {
		dbPath = filepath.Join(*dataDir, "panel.db")
	}
	store, err := world.OpenStore(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()

	host := *dockerHost
	if host == "" {
		host = os.Getenv("DOCKER_HOST")
	}
	runtime, err := world.NewDocker(host)
	if err != nil {
		log.Fatal(err)
	}

	svc := world.NewService(store, runtime, nil, world.NewHTTPGateway(*gatewayURL), *dataDir, *domain)
	log.Printf("panel listening on %s (sqlite %s)", *addr, dbPath)
	if err := http.ListenAndServe(*addr, world.Handler{Service: svc}.Mux()); err != nil {
		log.Fatal(err)
	}
}
