package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memorySnapshotter struct {
	err   error
	calls []Request
}

func (m *memorySnapshotter) Create(_ context.Context, req Request) (Snapshot, error) {
	m.calls = append(m.calls, req)
	if m.err != nil {
		return Snapshot{}, m.err
	}
	return Snapshot{ID: "snap-1", VolumeID: req.VolumeID, Description: req.Description}, nil
}

func TestDeleteSaveKeepsDirectoryByDefault(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "world")
	if err := os.Mkdir(save, 0o755); err != nil {
		t.Fatal(err)
	}
	snap := &memorySnapshotter{}
	svc := &Service{Snapshots: snap}

	err := svc.DeleteSave(context.Background(), DeleteSaveRequest{
		WorldID:  "survival",
		SaveDir:  save,
		VolumeID: "vol-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.calls) != 0 {
		t.Fatalf("snapshot calls = %d, want 0 when keep is default", len(snap.calls))
	}
	if _, err := os.Stat(save); err != nil {
		t.Fatalf("save directory should remain: %v", err)
	}
}

func TestDeleteSaveSnapshotsBeforeRemoving(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "world")
	if err := os.Mkdir(save, 0o755); err != nil {
		t.Fatal(err)
	}
	var removed []string
	snap := &memorySnapshotter{}
	svc := &Service{
		Snapshots: snap,
		RemoveAll: func(path string) error {
			if len(snap.calls) == 0 {
				t.Fatal("removed save before snapshot")
			}
			removed = append(removed, path)
			return os.RemoveAll(path)
		},
	}

	err := svc.DeleteSave(context.Background(), DeleteSaveRequest{
		WorldID:  "survival",
		SaveDir:  save,
		VolumeID: "vol-123",
		Destroy:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.calls) != 1 {
		t.Fatalf("snapshot calls = %d, want 1", len(snap.calls))
	}
	got := snap.calls[0]
	if got.VolumeID != "vol-123" {
		t.Fatalf("volume = %q", got.VolumeID)
	}
	if !strings.Contains(got.Description, "survival") {
		t.Fatalf("description = %q", got.Description)
	}
	if got.Tags["Purpose"] != preDeletePurpose || got.Tags["WorldID"] != "survival" {
		t.Fatalf("tags = %#v", got.Tags)
	}
	if len(removed) != 1 || removed[0] != save {
		t.Fatalf("removed = %#v", removed)
	}
	if _, err := os.Stat(save); !os.IsNotExist(err) {
		t.Fatalf("save should be gone, stat err = %v", err)
	}
}

func TestDeleteSaveUsesRemoveAllDefault(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "world")
	if err := os.Mkdir(save, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Snapshots: &memorySnapshotter{}}
	if err := svc.DeleteSave(context.Background(), DeleteSaveRequest{
		WorldID:  "nether",
		SaveDir:  save,
		VolumeID: "vol-9",
		Destroy:  true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(save); !os.IsNotExist(err) {
		t.Fatalf("default RemoveAll should delete, stat err = %v", err)
	}
}

func TestDeleteSaveStopsWhenSnapshotFails(t *testing.T) {
	dir := t.TempDir()
	save := filepath.Join(dir, "world")
	if err := os.Mkdir(save, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := &Service{Snapshots: &memorySnapshotter{err: errors.New("throttled")}}
	err := svc.DeleteSave(context.Background(), DeleteSaveRequest{
		WorldID:  "survival",
		SaveDir:  save,
		VolumeID: "vol-123",
		Destroy:  true,
	})
	if !errors.Is(err, ErrSnapshotFailed) {
		t.Fatalf("err = %v, want ErrSnapshotFailed", err)
	}
	if _, statErr := os.Stat(save); statErr != nil {
		t.Fatalf("save should remain after snapshot failure: %v", statErr)
	}
}

func TestDeleteSaveRequiresVolumeAndSafePath(t *testing.T) {
	svc := &Service{Snapshots: &memorySnapshotter{}}
	cases := []struct {
		name string
		req  DeleteSaveRequest
		want error
	}{
		{name: "missing dir", req: DeleteSaveRequest{Destroy: true, VolumeID: "vol-1"}, want: ErrMissingSaveDir},
		{name: "root", req: DeleteSaveRequest{Destroy: true, VolumeID: "vol-1", SaveDir: "/"}, want: ErrUnsafePath},
		{name: "dot", req: DeleteSaveRequest{Destroy: true, VolumeID: "vol-1", SaveDir: "."}, want: ErrUnsafePath},
		{name: "missing volume", req: DeleteSaveRequest{Destroy: true, SaveDir: "/data/worlds/a"}, want: ErrMissingVolume},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.DeleteSave(context.Background(), tc.req)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDeleteSaveRequiresSnapshotter(t *testing.T) {
	var svc *Service
	err := svc.DeleteSave(context.Background(), DeleteSaveRequest{
		Destroy:  true,
		SaveDir:  "/data/worlds/a",
		VolumeID: "vol-1",
	})
	if !errors.Is(err, ErrSnapshotFailed) {
		t.Fatalf("err = %v, want ErrSnapshotFailed", err)
	}

	svc = &Service{}
	err = svc.DeleteSave(context.Background(), DeleteSaveRequest{
		Destroy:  true,
		SaveDir:  "/data/worlds/a",
		VolumeID: "vol-1",
	})
	if !errors.Is(err, ErrSnapshotFailed) {
		t.Fatalf("nil snapshotter err = %v", err)
	}
}

func TestPointWorldAtRestoredDirectory(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "level.dat")
	if err := os.WriteFile(marker, []byte("save"), 0o644); err != nil {
		t.Fatal(err)
	}

	vol, err := PointWorldAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if vol.Path != dir {
		t.Fatalf("path = %q, want restored dir", vol.Path)
	}
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "save" {
		t.Fatal("restore must not rewrite the directory")
	}
}

func TestPointWorldAtRejectsMissingAndFiles(t *testing.T) {
	if _, err := PointWorldAt(""); !errors.Is(err, ErrMissingSaveDir) {
		t.Fatalf("empty: %v", err)
	}
	if _, err := PointWorldAt(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing directory should fail")
	}
	file := filepath.Join(t.TempDir(), "level.dat")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PointWorldAt(file); !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("file: %v", err)
	}
}

func TestCopyWorldIsUnsupported(t *testing.T) {
	if err := CopyWorld("/from", "/to"); !errors.Is(err, ErrCopyUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestVolumeIDFromEnv(t *testing.T) {
	t.Setenv(VolumeIDEnv, "  vol-abc  ")
	if got := VolumeIDFromEnv(); got != "vol-abc" {
		t.Fatalf("got %q", got)
	}
}
