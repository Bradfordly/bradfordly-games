package world

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPICreateGetPatchDelete(t *testing.T) {
	h := testHandler(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/worlds", bytes.NewBufferString(`{"name":"Survival"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.Bytes())
	}
	var created World
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Allocation.Host != "survival.games.bradfordly.com" {
		t.Fatalf("host = %q", created.Allocation.Host)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/worlds/"+created.ID, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/worlds", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPatch, "/api/worlds/"+created.ID, bytes.NewBufferString(`{"idle_timeout":"25m","env":{"DIFFICULTY":"hard"}}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", rec.Code, rec.Body.Bytes())
	}
	var patched World
	if err := json.Unmarshal(rec.Body.Bytes(), &patched); err != nil {
		t.Fatal(err)
	}
	if patched.Volume != created.Volume || patched.Env["DIFFICULTY"] != "hard" {
		t.Fatalf("patched = %#v", patched)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/worlds/"+created.ID, nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/worlds/"+created.ID, nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get after delete = %d", rec.Code)
	}
}

func TestAPICreateRejectsValheimAndBadJSON(t *testing.T) {
	h := testHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/worlds", bytes.NewBufferString(`{"name":"x","game":"valheim"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("valheim status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/worlds", bytes.NewBufferString(`{`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json status = %d", rec.Code)
	}
}

func TestAPIHealthz(t *testing.T) {
	h := testHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestAPIDeleteDestroy(t *testing.T) {
	svc, _, files, _ := testService(t)
	h := Handler{Service: svc}.Mux()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/worlds", bytes.NewBufferString(`{"name":"Survival"}`)))
	var created World
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/worlds/"+created.ID+"?destroy=true", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("destroy status = %d", rec.Code)
	}
	if files.Has(created.Volume) {
		t.Fatal("save should be destroyed")
	}
}

func TestAPIPatchBadJSONAndConflict(t *testing.T) {
	h := testHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/worlds", bytes.NewBufferString(`{"name":"Survival"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/worlds/fixedid1", bytes.NewBufferString(`{`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad patch json = %d", rec.Code)
	}
}

func TestAPIDeleteBadDestroy(t *testing.T) {
	h := testHandler(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/worlds/x?destroy=maybe", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	svc, _, _, _ := testService(t)
	return Handler{Service: svc}.Mux()
}
