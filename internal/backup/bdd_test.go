package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

type bddState struct {
	dir      string
	save     string
	volumeID string
	snap     *memorySnapshotter
	removed  []string
	svc      *Service
	world    WorldVolume
	err      error
}

func (s *bddState) reset(t *testing.T) {
	s.dir = t.TempDir()
	s.save = ""
	s.volumeID = "vol-57"
	s.snap = &memorySnapshotter{}
	s.removed = nil
	s.svc = &Service{
		Snapshots: s.snap,
		RemoveAll: func(path string) error {
			s.removed = append(s.removed, path)
			return os.RemoveAll(path)
		},
	}
	s.world = WorldVolume{}
	s.err = nil
}

func (s *bddState) aWorldSaveDirectoryExists() error {
	s.save = filepath.Join(s.dir, "worlds", "survival")
	if err := os.MkdirAll(s.save, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.save, "level.dat"), []byte("chunks"), 0o644)
}

func (s *bddState) theDataVolumeIDIsConfigured() error {
	s.volumeID = "vol-57"
	return nil
}

func (s *bddState) snapshotCreationWillFail() error {
	s.snap.err = errors.New("ebs unavailable")
	return nil
}

func (s *bddState) deleteWorld(destroy bool) error {
	s.err = s.svc.DeleteSave(context.Background(), DeleteSaveRequest{
		WorldID:  "survival",
		SaveDir:  s.save,
		VolumeID: s.volumeID,
		Destroy:  destroy,
	})
	return nil
}

func (s *bddState) operatorDestroysTheSave() error {
	return s.deleteWorld(true)
}

func (s *bddState) operatorKeepsTheSave() error {
	return s.deleteWorld(false)
}

func (s *bddState) onDemandSnapshotCreated() error {
	if s.err != nil {
		return s.err
	}
	if len(s.snap.calls) != 1 {
		return errors.New("expected one on-demand snapshot")
	}
	if s.snap.calls[0].VolumeID != s.volumeID {
		return errors.New("snapshot did not target the data volume")
	}
	return nil
}

func (s *bddState) saveRemovedAfterSnapshot() error {
	if len(s.removed) != 1 {
		return errors.New("save directory was not removed after snapshot")
	}
	if _, err := os.Stat(s.save); !os.IsNotExist(err) {
		return errors.New("save directory still exists")
	}
	return nil
}

func (s *bddState) saveStillPresent() error {
	if _, err := os.Stat(s.save); err != nil {
		return err
	}
	return nil
}

func (s *bddState) noSnapshotCreated() error {
	if len(s.snap.calls) != 0 {
		return errors.New("snapshot was created when the save was kept")
	}
	return nil
}

func (s *bddState) aRestoredSaveDirectoryExists() error {
	s.save = filepath.Join(s.dir, "restored", "survival")
	if err := os.MkdirAll(s.save, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.save, "level.dat"), []byte("restored"), 0o644)
}

func (s *bddState) createWorldPointedAtRestored() error {
	s.world, s.err = PointWorldAt(s.save)
	return nil
}

func (s *bddState) worldVolumeIsRestoredDir() error {
	if s.err != nil {
		return s.err
	}
	if s.world.Path != s.save {
		return errors.New("world volume is not the restored directory")
	}
	return nil
}

func (s *bddState) noCopyThroughPanelAPI() error {
	if err := CopyWorld(s.save, filepath.Join(s.dir, "copy")); !errors.Is(err, ErrCopyUnsupported) {
		return errors.New("panel copy API must stay unsupported")
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() == "copy" {
			return errors.New("copied a world directory")
		}
	}
	return nil
}

func backupsHCL() (string, error) {
	root := filepath.Join("..", "..", "infra", "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".tf") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return "", err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func weeklyDLMPolicyExists() error {
	text, err := backupsHCL()
	if err != nil {
		return err
	}
	for _, need := range []string{
		`resource "aws_dlm_lifecycle_policy"`,
		"WEEKS",
		"data-volume",
		"target_tags",
	} {
		if !strings.Contains(text, need) {
			return errors.New("missing " + need)
		}
	}
	return nil
}

func backupsModuleDoesNotCreateHost() error {
	text, err := backupsHCL()
	if err != nil {
		return err
	}
	for _, banned := range []string{"aws_instance", "aws_ebs_volume", "aws_eks_cluster"} {
		if strings.Contains(text, banned) {
			return errors.New("backups module created " + banned)
		}
	}
	return nil
}

func TestFeatures(t *testing.T) {
	state := &bddState{}
	suite := godog.TestSuite{
		Name: "backups",
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				state.reset(t)
				return ctx, nil
			})
			ctx.Step(`^a world save directory exists on the data volume$`, state.aWorldSaveDirectoryExists)
			ctx.Step(`^the data volume id is configured$`, state.theDataVolumeIDIsConfigured)
			ctx.Step(`^snapshot creation will fail$`, state.snapshotCreationWillFail)
			ctx.Step(`^the operator deletes the world and asks to destroy the save$`, state.operatorDestroysTheSave)
			ctx.Step(`^the operator deletes the world and keeps the save$`, state.operatorKeepsTheSave)
			ctx.Step(`^an on-demand snapshot of the data volume is created$`, state.onDemandSnapshotCreated)
			ctx.Step(`^the save directory is removed only after the snapshot is requested$`, state.saveRemovedAfterSnapshot)
			ctx.Step(`^the save directory is still present$`, state.saveStillPresent)
			ctx.Step(`^no snapshot is created$`, state.noSnapshotCreated)
			ctx.Step(`^a restored save directory exists$`, state.aRestoredSaveDirectoryExists)
			ctx.Step(`^the operator creates a new world pointed at that directory$`, state.createWorldPointedAtRestored)
			ctx.Step(`^the world volume is the restored directory$`, state.worldVolumeIsRestoredDir)
			ctx.Step(`^no world files are copied through the panel API$`, state.noCopyThroughPanelAPI)
			ctx.Step(`^a DLM policy retains weekly snapshots of volumes tagged Role=data-volume$`, weeklyDLMPolicyExists)
			ctx.Step(`^the backups module does not create the EC2 host or data volume$`, backupsModuleDoesNotCreateHost)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{filepath.Join("..", "..", "features")},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("bdd scenarios failed")
	}
}
