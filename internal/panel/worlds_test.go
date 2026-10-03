package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

func TestWorldsAPIRequiresSession(t *testing.T) {
	srv := testServer("bradfordly", Identity{})
	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/worlds", nil),
		httptest.NewRequest(http.MethodPost, "/api/worlds", strings.NewReader(`{"name":"X"}`)),
		httptest.NewRequest(http.MethodGet, "/api/worlds/x", nil),
		httptest.NewRequest(http.MethodPatch, "/api/worlds/x", strings.NewReader(`{"name":"Y"}`)),
	} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want %d", req.Method, req.URL.Path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestCreateGetPatchWorlds(t *testing.T) {
	srv := testServer("bradfordly", Identity{Login: "bradfordly"})
	cookie := authedCookie(t, srv)

	create := doJSON(t, srv, cookie, http.MethodPost, "/api/worlds", `{"name":"Survival","allocation":{"host":"survival.games.bradfordly.com"}}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("POST /api/worlds status = %d body=%s", create.Code, create.Body.Bytes())
	}
	var rec world.Record
	decodeJSON(t, create.Body, &rec)
	if rec.ID != "survival" || rec.IdleTimeout != "15m" || rec.OccupyMode != "kick" {
		t.Fatalf("created = %+v", rec)
	}
	if rec.StartTimeout != "10m" || rec.StopTimeout != "2m" {
		t.Fatalf("timeouts = %+v", rec)
	}
	if strings.Contains(create.Body.String(), "replica") {
		t.Fatal("API must not write replica counts")
	}

	listed := doJSON(t, srv, cookie, http.MethodGet, "/api/worlds", "")
	if listed.Code != http.StatusOK {
		t.Fatalf("GET /api/worlds status = %d", listed.Code)
	}
	var worlds []world.Record
	decodeJSON(t, listed.Body, &worlds)
	if len(worlds) != 1 || worlds[0].ID != "survival" {
		t.Fatalf("list = %+v", worlds)
	}

	got := doJSON(t, srv, cookie, http.MethodGet, "/api/worlds/survival", "")
	if got.Code != http.StatusOK {
		t.Fatalf("GET /api/worlds/survival status = %d", got.Code)
	}

	patched := doJSON(t, srv, cookie, http.MethodPatch, "/api/worlds/survival", `{"idle_timeout":"20m","asleep_motd":"asleep — join the game to start"}`)
	if patched.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d body=%s", patched.Code, patched.Body.Bytes())
	}
	decodeJSON(t, patched.Body, &rec)
	if rec.IdleTimeout != "20m" || rec.AsleepMOTD == "" || rec.Name != "Survival" {
		t.Fatalf("patched = %+v", rec)
	}

	tooShort := doJSON(t, srv, cookie, http.MethodPatch, "/api/worlds/survival", `{"idle_timeout":"30s"}`)
	if tooShort.Code != http.StatusBadRequest {
		t.Fatalf("short idle_timeout status = %d", tooShort.Code)
	}
}

func authedCookie(t *testing.T, srv *Server) *http.Cookie {
	t.Helper()
	return cookieFrom(t, callback(t, srv, "ok"), sessionCookie)
}

func doJSON(t *testing.T, srv *Server, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, r io.Reader, dest any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(dest); err != nil {
		t.Fatal(err)
	}
}
