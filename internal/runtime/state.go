package runtime

import (
	"sort"
	"sync"
	"time"
)

const (
	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"
	StatusOpen     = "open"
)

type ProviderRuntimeState struct {
	Name              string    `json:"name"`
	Status            string    `json:"status"`
	TotalRequests     uint64    `json:"total_requests"`
	TotalErrors       uint64    `json:"total_errors"`
	ConsecutiveErrors uint64    `json:"consecutive_errors"`
	LastErrorAt       time.Time `json:"last_error_at,omitempty"`
	LastSuccessAt     time.Time `json:"last_success_at,omitempty"`
	WindowErrors      uint64    `json:"window_errors"`
	LatencyMS         int64     `json:"latency_ms"`
	window            []time.Time
}

type Store struct {
	mu     sync.RWMutex
	states map[string]*ProviderRuntimeState
	window time.Duration
}

func NewStore(names []string, window time.Duration) *Store {
	states := make(map[string]*ProviderRuntimeState, len(names))
	for _, name := range names {
		states[name] = &ProviderRuntimeState{Name: name, Status: StatusHealthy}
	}
	return &Store{states: states, window: window}
}

func (s *Store) Report(name string, success bool, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensure(name)
	state.TotalRequests++
	state.LatencyMS = latency.Milliseconds()
	if success {
		state.ConsecutiveErrors = 0
		state.LastSuccessAt = time.Now()
		state.Status = StatusHealthy
		return
	}
	now := time.Now()
	state.TotalErrors++
	state.ConsecutiveErrors++
	state.LastErrorAt = now
	state.window = append(state.window, now)
	state.WindowErrors = uint64(len(trimWindow(state.window, s.window, now)))
	state.window = trimWindow(state.window, s.window, now)
	if state.ConsecutiveErrors >= 3 {
		state.Status = StatusDegraded
	}
}

func (s *Store) Snapshot(name string) ProviderRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.ensure(name)
	now := time.Now()
	state.window = trimWindow(state.window, s.window, now)
	state.WindowErrors = uint64(len(state.window))
	return copyState(state)
}

func (s *Store) All() []ProviderRuntimeState {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := make([]ProviderRuntimeState, 0, len(s.states))
	for _, state := range s.states {
		state.window = trimWindow(state.window, s.window, now)
		state.WindowErrors = uint64(len(state.window))
		out = append(out, copyState(state))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (s *Store) ensure(name string) *ProviderRuntimeState {
	state, ok := s.states[name]
	if !ok {
		state = &ProviderRuntimeState{Name: name, Status: StatusHealthy}
		s.states[name] = state
	}
	return state
}

func trimWindow(in []time.Time, window time.Duration, now time.Time) []time.Time {
	if window <= 0 || len(in) == 0 {
		return in
	}
	cutoff := now.Add(-window)
	idx := 0
	for idx < len(in) && in[idx].Before(cutoff) {
		idx++
	}
	return in[idx:]
}

func copyState(s *ProviderRuntimeState) ProviderRuntimeState {
	return ProviderRuntimeState{
		Name:              s.Name,
		Status:            s.Status,
		TotalRequests:     s.TotalRequests,
		TotalErrors:       s.TotalErrors,
		ConsecutiveErrors: s.ConsecutiveErrors,
		LastErrorAt:       s.LastErrorAt,
		LastSuccessAt:     s.LastSuccessAt,
		WindowErrors:      s.WindowErrors,
		LatencyMS:         s.LatencyMS,
	}
}
