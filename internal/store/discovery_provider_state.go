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
	state, err := s.Load(name)
	if err != nil && !errors.Is(err, ErrDiscoveryProviderStateNotFound) {
		return DiscoveryProviderState{}, "", err
	}
	if errors.Is(err, ErrDiscoveryProviderStateNotFound) {
		state = DiscoveryProviderState{Name: name}
	}

	now := s.now().UTC()
	state.Name = name
	state.UpdatedAt = now
	state.LastOutcome = strings.TrimSpace(observation.Outcome)
	cooldown := time.Duration(0)

	switch state.LastOutcome {
	case DiscoveryProviderOutcomeSuccess:
		state.SuccessCount++
		state.ConsecutiveFailures = 0
		state.CooldownUntil = time.Time{}
		state.LastSuccessAt = now
	case DiscoveryProviderOutcomeBlocked:
		state.BlockedCount++
		state.ConsecutiveFailures++
		state.LastBlockedAt = now
		state.LastFailureAt = now
		cooldown = observation.BlockedCooldown
	case DiscoveryProviderOutcomeTimeout:
		state.TimeoutCount++
		state.ConsecutiveFailures++
		state.LastTimeoutAt = now
		state.LastFailureAt = now
		cooldown = observation.TimeoutCooldown
	case DiscoveryProviderOutcomeUnavailable:
		state.UnavailableCount++
		state.ConsecutiveFailures++
		state.LastFailureAt = now
		cooldown = observation.UnavailableCooldown
	case DiscoveryProviderOutcomeFailure:
		state.FailureCount++
		state.ConsecutiveFailures++
		state.LastFailureAt = now
		cooldown = observation.FailureCooldown
	default:
		return DiscoveryProviderState{}, "", fmt.Errorf("invalid discovery provider outcome %q", observation.Outcome)
	}
	if cooldown > 0 {
		until := now.Add(cooldown)
		if until.After(state.CooldownUntil) {
			state.CooldownUntil = until
		}
	}

	if err := state.Validate(); err != nil {
		return DiscoveryProviderState{}, "", err
	}
	dir := filepath.Join(s.root, "discovery", "providers")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DiscoveryProviderState{}, "", fmt.Errorf("create discovery provider dir: %w", err)
	}
	path := filepath.Join(dir, sanitizeID(name)+".json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return DiscoveryProviderState{}, "", fmt.Errorf("encode discovery provider state: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return DiscoveryProviderState{}, "", fmt.Errorf("write discovery provider state: %w", err)
	}
	return state, path, nil
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
