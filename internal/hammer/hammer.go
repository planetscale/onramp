// Package hammer
package hammer

import (
	"sync"
	"sync/atomic"

	"onramp/internal/hammer/htypes"
)

// SimulationState holds the in-memory simulation configuration.
// All access is protected by mu.
type SimulationState struct {
	mu           sync.RWMutex
	UserConfig   htypes.UserConfig
	Running      atomic.Bool
	Provider     string
	CurrentUsers atomic.Int64
}

func (s *SimulationState) GetUserConfig() htypes.UserConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UserConfig
}

func (s *SimulationState) SetUserConfig(uc htypes.UserConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UserConfig = uc
}

func (s *SimulationState) GetFlows() []htypes.Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]htypes.Flow, len(s.UserConfig.Flows))
	copy(result, s.UserConfig.Flows)
	return result
}

func (s *SimulationState) SetFlows(flows []htypes.Flow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.UserConfig.Flows = flows
}

func (s *SimulationState) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Running.Load()
}

func (s *SimulationState) SetRunning(r bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running.Store(r)
}

func (s *SimulationState) GetUsers() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int(s.CurrentUsers.Load())
}
