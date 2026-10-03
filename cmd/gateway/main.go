package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("admin-addr", ":8080", "admin HTTP listen address")
	flag.Parse()

	log.Printf("gateway admin listening on %s", *addr)
	if err := http.ListenAndServe(*addr, adminMux()); err != nil {
		log.Fatal(err)
	}
}
