package main

import (
	"flag"
	"log"

	"github.com/bradfordly/bradfordly-games/internal/gateway"
)

func main() {
	adminAddr := flag.String("admin-addr", ":8080", "admin HTTP listen address")
	gameAddr := flag.String("game-addr", ":25565", "Minecraft Java listen address")
	flag.Parse()

	srv := gateway.New(*adminAddr, *gameAddr)
	admin, game, err := srv.Start()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("gateway admin listening on %s", admin)
	log.Printf("gateway minecraft listening on %s", game)
	select {}
}
