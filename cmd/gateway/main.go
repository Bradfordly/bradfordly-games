package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func main() {
	addr := flag.String("admin-addr", ":8080", "admin HTTP listen address")
	flag.Parse()

	adapters := adapter.NewRegistry()
	log.Printf("gateway adapters: %s, %s, %s", adapter.GameMinecraftJava, adapter.GameValheim, adapter.GamePalworld)
	_ = adapters

	log.Printf("gateway admin listening on %s", *addr)
	if err := http.ListenAndServe(*addr, adminMux()); err != nil {
		log.Fatal(err)
	}
}
