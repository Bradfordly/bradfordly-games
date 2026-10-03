package panel

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

func TestWorldListAndDetailPages(t *testing.T) {
	srv := testServer("bradfordly", Identity{Login: "bradfordly"})
	players := 2
	srv.cfg.States = MemoryStates{ByID: map[string]world.Runtime{
		"survival": {State: "asleep"},
		"broken":   {State: "failed", LastError: "start timed out", PlayerCount: &players},
	}}
	cookie := authedCookie(t, srv)
	_, err := srv.cfg.Store.Create(nil, world.Record{
		ID: "survival", Name: "Survival", Game: world.GameMinecraftJava,
		Allocation: world.Allocation{Host: "survival.games.bradfordly.com", Port: 25565},
	}.World())
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.cfg.Store.Create(nil, world.Record{
		ID: "broken", Name: "Broken", Game: world.GameMinecraftJava,
	}.World())
	if err != nil {
		t.Fatal(err)
	}

	list := doJSON(t, srv, cookie, http.MethodGet, "/", "")
	if list.Code != http.StatusOK {
		t.Fatalf("GET / status = %d", list.Code)
	}
	body := list.Body.String()
	if !strings.Contains(body, "asleep — join the game to start") {
		t.Fatalf("list missing asleep copy: %s", body)
	}
	if strings.Contains(body, "class=\"error\"") && strings.Contains(body, "asleep") {
		t.Fatal("asleep must not be an error")
	}
	if !strings.Contains(body, "failed") || !strings.Contains(body, "start timed out") {
		t.Fatalf("list must show failed: %s", body)
	}
	if !strings.Contains(body, "survival.games.bradfordly.com:25565") {
		t.Fatalf("list missing allocation: %s", body)
	}

	detail := doJSON(t, srv, cookie, http.MethodGet, "/worlds/broken", "")
	if detail.Code != http.StatusOK {
		t.Fatalf("GET /worlds/broken status = %d", detail.Code)
	}
	dbody := detail.Body.String()
	if !strings.Contains(dbody, "failed") || !strings.Contains(dbody, "start timed out") {
		t.Fatalf("detail must show failed: %s", dbody)
	}
	if !strings.Contains(dbody, "Players are online. Stop anyway?") {
		t.Fatalf("detail must confirm stop while players are online: %s", dbody)
	}
	if !strings.Contains(dbody, "/api/worlds/"+"broken"+"/power") {
		t.Fatalf("detail missing power action: %s", dbody)
	}
}
