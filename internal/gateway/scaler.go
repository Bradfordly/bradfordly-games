package gateway

import "sync"

// Scaler changes a world's replica count. Kubernetes arrives later; tests use a recorder.
type Scaler interface {
	SetReplicas(worldID string, replicas int) error
}

type RecordingScaler struct {
	mu    sync.Mutex
	Calls []ScaleCall
}

type ScaleCall struct {
	WorldID  string
	Replicas int
}

func (s *RecordingScaler) SetReplicas(worldID string, replicas int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, ScaleCall{WorldID: worldID, Replicas: replicas})
	return nil
}

func (s *RecordingScaler) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.Calls)
}
