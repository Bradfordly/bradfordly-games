package panel

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

func TestEnsureSleepingMinecraft(t *testing.T) {
	cluster := NewMemoryCluster()
	rec := world.Record{
		ID:   "survival",
		Name: "Survival",
		Game: world.GameMinecraftJava,
		Allocation: world.Allocation{
			Host: "survival.games.bradfordly.com",
			Port: 25565,
		},
	}
	world.ApplyDefaults(&rec)
	r := NewSleepingReconciler(cluster, "worlds", "")
	if err := r.EnsureSleeping(context.Background(), rec.World()); err != nil {
		t.Fatal(err)
	}

	svc, ok := cluster.Services["survival"]
	if !ok {
		t.Fatal("missing ClusterIP service")
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("service type = %s", svc.Spec.Type)
	}

	sts, ok := cluster.StatefulSets["survival"]
	if !ok {
		t.Fatal("missing StatefulSet")
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Fatalf("replicas = %v, want 0", sts.Spec.Replicas)
	}
	if sts.Name != sts.Spec.ServiceName {
		t.Fatalf("StatefulSet name %q != serviceName %q", sts.Name, sts.Spec.ServiceName)
	}
	ctr := sts.Spec.Template.Spec.Containers[0]
	if ctr.Ports[0].HostPort != 0 {
		t.Fatalf("HostPort = %d, want unset", ctr.Ports[0].HostPort)
	}
	if ctr.Image != world.DefaultImage {
		t.Fatalf("image = %s", ctr.Image)
	}
	if ctr.VolumeMounts[0].MountPath != minecraftMountPath {
		t.Fatalf("mount = %s", ctr.VolumeMounts[0].MountPath)
	}

	pvc, ok := cluster.PVCs["survival-data"]
	if !ok {
		t.Fatal("missing PVC")
	}
	pv, ok := cluster.PVs[pvc.Spec.VolumeName]
	if !ok {
		t.Fatal("missing PV")
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != efsCSIDriver {
		t.Fatalf("pv csi = %+v", pv.Spec.CSI)
	}
	if pv.Spec.CSI.VolumeHandle != "fs-example::fsap-survival" {
		t.Fatalf("volumeHandle = %s, want documented example convention", pv.Spec.CSI.VolumeHandle)
	}
}

func TestCreateWorldReconcilesSleepingSTS(t *testing.T) {
	cluster := NewMemoryCluster()
	srv := testServer("bradfordly", Identity{Login: "bradfordly"})
	srv.cfg.Reconcile = NewSleepingReconciler(cluster, "worlds", "fs-example")
	cookie := authedCookie(t, srv)
	rec := doJSON(t, srv, cookie, "POST", "/api/worlds", `{"name":"Survival"}`)
	if rec.Code != 201 {
		t.Fatalf("create status = %d body=%s", rec.Code, rec.Body.Bytes())
	}
	sts := cluster.StatefulSets["survival"]
	if sts == nil || sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Fatalf("create must leave the world asleep: %+v", sts)
	}
}
