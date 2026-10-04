// Package backup implements the v1 backup minimum from docs/specs/operations.md:
// an on-demand snapshot of the #57 data volume before a world's save directory
// is deleted, and restore as a new world pointed at a restored directory.
package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// VolumeIDEnv is the panel environment variable for the 50 GB data volume.
	// #57 sets this after the volume exists.
	VolumeIDEnv = "GAMES_DATA_VOLUME_ID"

	preDeletePurpose = "pre-delete"
)

var (
	ErrMissingVolume   = errors.New("data volume id is required")
	ErrMissingSaveDir  = errors.New("save directory is required")
	ErrUnsafePath      = errors.New("refusing to delete an unsafe save path")
	ErrSnapshotFailed  = errors.New("on-demand snapshot failed")
	ErrNotDirectory    = errors.New("restored path is not a directory")
	ErrCopyUnsupported = errors.New("copying worlds through the panel API is not supported")
)

// Request is an on-demand snapshot of the data volume.
type Request struct {
	VolumeID    string
	Description string
	Tags        map[string]string
}

// Snapshot is the identifier returned after CreateSnapshot is accepted.
type Snapshot struct {
	ID          string
	VolumeID    string
	Description string
}

// Snapshotter creates a point-in-time EBS snapshot. The snapshot captures the
// volume when the API call is accepted; callers may then delete a save directory.
type Snapshotter interface {
	Create(ctx context.Context, req Request) (Snapshot, error)
}

// DeleteSaveRequest is the control-plane delete path.
// Destroy false is the default in control-plane.md: keep the save directory.
type DeleteSaveRequest struct {
	WorldID  string
	SaveDir  string
	VolumeID string
	Destroy  bool
}

// WorldVolume is the host path bind-mounted into a world container.
type WorldVolume struct {
	Path string
}

// Service snapshots the data volume, then removes a save directory.
type Service struct {
	Snapshots Snapshotter
	RemoveAll func(path string) error
}

// VolumeIDFromEnv reads the data volume id #57 wires into the panel.
func VolumeIDFromEnv() string {
	return strings.TrimSpace(os.Getenv(VolumeIDEnv))
}

// DeleteSave keeps the directory unless Destroy is set. A destroy always
// snapshots first; a snapshot error leaves the directory in place.
func (s *Service) DeleteSave(ctx context.Context, req DeleteSaveRequest) error {
	if !req.Destroy {
		return nil
	}
	if err := validateSaveDir(req.SaveDir); err != nil {
		return err
	}
	if strings.TrimSpace(req.VolumeID) == "" {
		return ErrMissingVolume
	}
	if s == nil || s.Snapshots == nil {
		return ErrSnapshotFailed
	}

	_, err := s.Snapshots.Create(ctx, Request{
		VolumeID:    req.VolumeID,
		Description: fmt.Sprintf("bradfordly-games pre-delete world=%s", req.WorldID),
		Tags: map[string]string{
			"Project": "bradfordly-games",
			"Purpose": preDeletePurpose,
			"WorldID": req.WorldID,
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshotFailed, err)
	}

	remove := s.RemoveAll
	if remove == nil {
		remove = os.RemoveAll
	}
	return remove(req.SaveDir)
}

// PointWorldAt uses a restored directory as a new world's volume.
// It does not copy files.
func PointWorldAt(dir string) (WorldVolume, error) {
	if strings.TrimSpace(dir) == "" {
		return WorldVolume{}, ErrMissingSaveDir
	}
	info, err := os.Stat(dir)
	if err != nil {
		return WorldVolume{}, err
	}
	if !info.IsDir() {
		return WorldVolume{}, ErrNotDirectory
	}
	return WorldVolume{Path: dir}, nil
}

// CopyWorld is rejected. Restore is a new world pointed at a restored directory.
func CopyWorld(fromDir, toDir string) error {
	return ErrCopyUnsupported
}

func validateSaveDir(path string) error {
	if strings.TrimSpace(path) == "" {
		return ErrMissingSaveDir
	}
	cleaned := filepath.Clean(path)
	if cleaned == "/" || cleaned == "." {
		return ErrUnsafePath
	}
	return nil
}
