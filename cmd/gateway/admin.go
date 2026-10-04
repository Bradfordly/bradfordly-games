package main

import (
	"net/http"

	"github.com/bradfordly/bradfordly-games/internal/gateway"
)

func adminMux() http.Handler {
	return gateway.New("127.0.0.1:0", "127.0.0.1:0").AdminHandler()
}
