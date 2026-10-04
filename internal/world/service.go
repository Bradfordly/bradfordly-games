package world

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// Service reconciles SQLite, the save directory, and a stopped container.
type Service struct {
	Store   *Store
	Runtime Runtime
	Files   Files
	Gateway Gateway
	DataDir string
	Domain  string
	newID   func() (string, error)
}

// NewService wires the control-plane reconcile path.
func NewService(store *Store, runtime Runtime, files Files, gateway Gateway, dataDir, domain string) *Service {
	if files == nil {
		files = diskFiles{}
	}
	if domain == "" {
		domain = DefaultDomain
	}
	return &Service{
		Store:   store,
		Runtime: runtime,
		Files:   files,
		Gateway: gateway,
		DataDir: dataDir,
		Domain:  domain,
		newID:   randomID,
	}
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (World, error) {
	id, err := s.newID()
	if err != nil {
		return World{}, err
	}
	w, err := normalizeCreate(req, id, s.DataDir, s.Domain)
	if err != nil {
		return World{}, err
	}
	if err := s.Files.MkdirAll(w.Volume, 0o755); err != nil {
		return World{}, fmt.Errorf("save directory: %w", err)
	}
	spec, err := containerSpec(w)
	if err != nil {
		return World{}, err
	}
	_, inspectErr := s.Runtime.Inspect(ctx, w.Backend.Container)
	createdContainer := inspectErr != nil
	if err := s.Runtime.EnsureStopped(ctx, spec); err != nil {
		return World{}, fmt.Errorf("container: %w", err)
	}
	if err := s.Store.Insert(w); err != nil {
		if createdContainer {
			_ = s.Runtime.Remove(ctx, w.Backend.Container)
		}
		return World{}, err
	}
	if s.Gateway != nil {
		_ = s.Gateway.Reload(ctx)
	}
	return w, nil
}

func (s *Service) Get(id string) (World, error) {
	return s.Store.Get(id)
}

func (s *Service) List() ([]World, error) {
	return s.Store.List()
}

func (s *Service) Update(ctx context.Context, id string, req UpdateRequest) (World, error) {
	current, err := s.Store.Get(id)
	if err != nil {
		return World{}, err
	}
	volume := current.Volume
	updated, err := applyUpdate(current, req)
	if err != nil {
		return World{}, err
	}
	updated.Volume = volume
	spec, err := containerSpec(updated)
	if err != nil {
		return World{}, err
	}
	if err := s.Runtime.EnsureStopped(ctx, spec); err != nil {
		return World{}, fmt.Errorf("container: %w", err)
	}
	if err := s.Store.Update(updated); err != nil {
		return World{}, err
	}
	if s.Gateway != nil {
		_ = s.Gateway.Reload(ctx)
	}
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, id string, destroy bool) error {
	w, err := s.Store.Get(id)
	if err != nil {
		return err
	}
	if s.Gateway != nil {
		if err := s.Gateway.Drain(ctx, id); err != nil {
			return fmt.Errorf("drain: %w", err)
		}
	}
	if err := s.Runtime.Remove(ctx, w.Backend.Container); err != nil {
		return fmt.Errorf("container: %w", err)
	}
	if err := s.Store.Delete(id); err != nil {
		return err
	}
	if destroy {
		if err := s.Files.RemoveAll(w.Volume); err != nil {
			return fmt.Errorf("save directory: %w", err)
		}
	}
	if s.Gateway != nil {
		_ = s.Gateway.Reload(ctx)
	}
	return nil
}

func randomID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
