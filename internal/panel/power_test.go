package panel

import (
	"net/http"
	"testing"
)

func TestPowerRequiresSession(t *testing.T) {
	srv := testServer("bradfordly", Identity{})
	rec := doJSON(t, srv, &http.Cookie{Name: "x", Value: "y"}, http.MethodPost, "/api/worlds/survival/power", `{"action":"start"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated power status = %d", rec.Code)
	}
}

func TestPowerForwardsToGateway(t *testing.T) {
	gw := &MemoryGateway{}
	cluster := NewMemoryCluster()
	srv := testServer("bradfordly", Identity{Login: "bradfordly"})
	srv.cfg.Gateway = gw
	srv.cfg.Reconcile = NewSleepingReconciler(cluster, "worlds", "fs-example")
	cookie := authedCookie(t, srv)

	created := doJSON(t, srv, cookie, http.MethodPost, "/api/worlds", `{"name":"Survival"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d", created.Code)
	}

	start := doJSON(t, srv, cookie, http.MethodPost, "/api/worlds/survival/power", `{"action":"start"}`)
	if start.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", start.Code, start.Body.Bytes())
	}
	stop := doJSON(t, srv, cookie, http.MethodPost, "/api/worlds/survival/power", `{"action":"stop"}`)
	if stop.Code != http.StatusOK {
		t.Fatalf("stop status = %d", stop.Code)
	}
	if len(gw.Calls) != 2 || gw.Calls[0].Action != "start" || gw.Calls[1].Action != "stop" {
		t.Fatalf("gateway calls = %+v", gw.Calls)
	}
	sts := cluster.StatefulSets["survival"]
	if sts == nil || sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Fatalf("panel must not patch replicas: %+v", sts)
	}

	bad := doJSON(t, srv, cookie, http.MethodPost, "/api/worlds/survival/power", `{"action":"wake"}`)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad action status = %d", bad.Code)
	}
	missing := doJSON(t, srv, cookie, http.MethodPost, "/api/worlds/nope/power", `{"action":"start"}`)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing world status = %d", missing.Code)
	}
}
