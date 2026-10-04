package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bradfordly/bradfordly-games/internal/panel"
	"github.com/bradfordly/bradfordly-games/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print version and exit")
	addr := flag.String("addr", getenv("PANEL_ADDR", ":8080"), "HTTP listen address")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String)
		return
	}

	log.Printf("panel %s listening on %s", version.String, *addr)
	if err := http.ListenAndServe(*addr, panel.Handler()); err != nil {
		log.Fatal(err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
