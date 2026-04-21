package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

var ErrDiscoveryProviderStateNotFound = errors.New("discovery provider state not found")

const (
	DiscoveryProviderOutcomeSuccess     = "success"
	DiscoveryProviderOutcomeBlocked     = "blocked"
	DiscoveryProviderOutcomeTimeout     = "timeout"
	DiscoveryProviderOutcomeFailure     = "failure"
	DiscoveryProviderOutcomeUnavailable = "unavailable"
)

type DiscoveryProviderState struct {
	Name                string    `json:"name"`
	SuccessCount        int       `json:"success_count"`
	BlockedCount        int       `json:"blocked_count"`
	TimeoutCount        int       `json:"timeout_count"`
	FailureCount        int       `json:"failure_count"`
	UnavailableCount    int       `json:"unavailable_count"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	LastOutcome         string    `json:"last_outcome,omitempty"`
	CooldownUntil       time.Time `json:"cooldown_until,omitempty"`
	LastSuccessAt       time.Time `json:"last_success_at,omitempty"`
	LastFailureAt       time.Time `json:"last_failure_at,omitempty"`
	LastBlockedAt       time.Time `json:"last_blocked_at,omitempty"`
	LastTimeoutAt       time.Time `json:"last_timeout_at,omitempty"`
	UpdatedAt           time.Time `json:"updated_at,omitempty"`
}

type DiscoveryProviderObservation struct {
	Name                string
	Outcome             string
	FailureCooldown     time.Duration
	BlockedCooldown     time.Duration
	TimeoutCooldown     time.Duration
	UnavailableCooldown time.Duration
}

type DiscoveryProviderStateStore struct {
	root string
	now  func() time.Time
}

func NewDiscoveryProviderStateStore(root string) DiscoveryProviderStateStore {
	if strings.TrimSpace(root) == "" {
		root = platform.DefaultStateRoot()
	}
	return DiscoveryProviderStateStore{
		root: root,
		now:  time.Now,
	}
}

func (s DiscoveryProviderStateStore) Load(name string) (DiscoveryProviderState, error) {
	path := filepath.Join(s.root, "discovery", "providers", sanitizeID(name)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DiscoveryProviderState{}, fmt.Errorf("%w: %s", ErrDiscoveryProviderStateNotFound, name)
		}
		return DiscoveryProviderState{}, fmt.Errorf("read discovery provider state: %w", err)
	}
	var state DiscoveryProviderState
	if err := json.Unmarshal(data, &state); err != nil {
		return DiscoveryProviderState{}, fmt.Errorf("decode discovery provider state: %w", err)
	}
	if err := state.Validate(); err != nil {
		return DiscoveryProviderState{}, err
	}
	return state, nil
}

func (s DiscoveryProviderStateStore) Observe(observation DiscoveryProviderObservation) (DiscoveryProviderState, string, error) {
	name := strings.TrimSpace(observation.Name)
	if name == "" {
		return DiscoveryProviderState{}, "", fmt.Errorf("discovery provider name is required")
	}
	state, err := s.loadOrNewProviderState(name)
	if err != nil {
		return DiscoveryProviderState{}, "", err
	}
	now := s.now().UTC()
	if err := state.applyObservation(observation, now); err != nil {
		return DiscoveryProviderState{}, "", err
	}
	if err := state.Validate(); err != nil {
		return DiscoveryProviderState{}, "", err
	}
	path, err := s.saveProviderState(state)
	return state, path, err
}

func (s DiscoveryProviderStateStore) loadOrNewProviderState(name string) (DiscoveryProviderState, error) {
	state, err := s.Load(name)
	if err == nil {
		return state, nil
	}
	if errors.Is(err, ErrDiscoveryProviderStateNotFound) {
		return DiscoveryProviderState{Name: name}, nil
	}
	return DiscoveryProviderState{}, err
}

func (s DiscoveryProviderStateStore) saveProviderState(state DiscoveryProviderState) (string, error) {
	dir := filepath.Join(s.root, "discovery", "providers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create discovery provider dir: %w", err)
	}
	path := filepath.Join(dir, sanitizeID(state.Name)+".json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode discovery provider state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write discovery provider state: %w", err)
	}
	return path, nil
}

func (s *DiscoveryProviderState) applyObservation(observation DiscoveryProviderObservation, now time.Time) error {
	s.Name = strings.TrimSpace(observation.Name)
	s.UpdatedAt = now
	s.LastOutcome = strings.TrimSpace(observation.Outcome)
	cooldown, err := s.recordOutcome(now, observation)
	if err != nil {
		return err
	}
	s.applyCooldown(now, cooldown)
	return nil
}

func (s *DiscoveryProviderState) recordOutcome(now time.Time, observation DiscoveryProviderObservation) (time.Duration, error) {
	switch s.LastOutcome {
	case DiscoveryProviderOutcomeSuccess:
		s.SuccessCount++
		s.ConsecutiveFailures = 0
		s.CooldownUntil = time.Time{}
		s.LastSuccessAt = now
		return 0, nil
	case DiscoveryProviderOutcomeBlocked:
		s.BlockedCount++
		s.recordFailure(now)
		s.LastBlockedAt = now
		return observation.BlockedCooldown, nil
	case DiscoveryProviderOutcomeTimeout:
		s.TimeoutCount++
		s.recordFailure(now)
		s.LastTimeoutAt = now
		return observation.TimeoutCooldown, nil
	case DiscoveryProviderOutcomeUnavailable:
		s.UnavailableCount++
		s.recordFailure(now)
		return observation.UnavailableCooldown, nil
	case DiscoveryProviderOutcomeFailure:
		s.FailureCount++
		s.recordFailure(now)
		return observation.FailureCooldown, nil
	default:
		return 0, fmt.Errorf("invalid discovery provider outcome %q", observation.Outcome)
	}
}

func (s *DiscoveryProviderState) recordFailure(now time.Time) {
	s.ConsecutiveFailures++
	s.LastFailureAt = now
}

func (s *DiscoveryProviderState) applyCooldown(now time.Time, cooldown time.Duration) {
	if cooldown <= 0 {
		return
	}
	until := now.Add(cooldown)
	if until.After(s.CooldownUntil) {
		s.CooldownUntil = until
	}
}

func (s DiscoveryProviderState) Validate() error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("name is required")
	}
	for field, value := range map[string]int{
		"success_count":        s.SuccessCount,
		"blocked_count":        s.BlockedCount,
		"timeout_count":        s.TimeoutCount,
		"failure_count":        s.FailureCount,
		"unavailable_count":    s.UnavailableCount,
		"consecutive_failures": s.ConsecutiveFailures,
	} {
		if value < 0 {
			return fmt.Errorf("%s must be >= 0", field)
		}
	}
	switch strings.TrimSpace(s.LastOutcome) {
	case "", DiscoveryProviderOutcomeSuccess, DiscoveryProviderOutcomeBlocked, DiscoveryProviderOutcomeTimeout, DiscoveryProviderOutcomeFailure, DiscoveryProviderOutcomeUnavailable:
	default:
		return fmt.Errorf("last_outcome has invalid value %q", s.LastOutcome)
	}
	if !s.UpdatedAt.IsZero() && s.UpdatedAt.Location() != time.UTC {
		return fmt.Errorf("updated_at must be stored in UTC")
	}
	return nil
}

func (s DiscoveryProviderState) CoolingDown(at time.Time) bool {
	return !s.CooldownUntil.IsZero() && s.CooldownUntil.After(at.UTC())
}

func (s DiscoveryProviderState) HealthScore(at time.Time) float64 {
	score := float64(s.SuccessCount*3) -
		float64(s.BlockedCount*6) -
		float64(s.TimeoutCount*3) -
		float64(s.FailureCount*2) -
		float64(s.UnavailableCount*2) -
		float64(s.ConsecutiveFailures*2)
	if s.CoolingDown(at) {
		score -= 20
	}
	if s.LastOutcome == DiscoveryProviderOutcomeSuccess {
		score += 2
	}
	return score
}
