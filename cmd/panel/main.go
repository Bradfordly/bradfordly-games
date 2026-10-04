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
	flag.Parse()
	if *showVersion {
		fmt.Println(version.String)
		os.Exit(0)
	}

	opts, err := panel.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	srv, err := panel.New(opts)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("panel listening on %s", opts.Addr)
	log.Fatal(http.ListenAndServe(opts.Addr, srv))
}
