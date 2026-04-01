package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

var ErrGenomeNotFound = errors.New("genome not found")

type DomainGenome struct {
	Domain           string    `json:"domain"`
	ForceLane        int       `json:"force_lane"`
	PreferredProfile string    `json:"preferred_profile,omitempty"`
	PruningProfile   string    `json:"pruning_profile,omitempty"`
	RenderNeeded     bool      `json:"render_needed,omitempty"`
	NoiseLevel       string    `json:"noise_level,omitempty"`
	LastPageType     string    `json:"last_page_type,omitempty"`
	SeenCount        int       `json:"seen_count"`
	LastSeenAt       time.Time `json:"last_seen_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type GenomeObservation struct {
	URL              string
	ObservedLane     int
	PreferredProfile string
	PruningProfile   string
	RenderNeeded     bool
	FetchMode        string
	NoiseLevel       string
	PageType         string
}

type GenomeStore struct {
	root string
	now  func() time.Time
}

func NewGenomeStore(root string) GenomeStore {
	if strings.TrimSpace(root) == "" {
		root = platform.DefaultStateRoot()
	}
	return GenomeStore{
		root: root,
		now:  time.Now,
	}
}

func (s GenomeStore) LoadByURL(rawURL string) (DomainGenome, error) {
	domain, err := domainFromURL(rawURL)
	if err != nil {
		return DomainGenome{}, err
	}
	return s.Load(domain)
}

func (s GenomeStore) Load(domain string) (DomainGenome, error) {
	path := filepath.Join(s.root, "genome", sanitizeID(domain)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DomainGenome{}, fmt.Errorf("%w: %s", ErrGenomeNotFound, domain)
		}
		return DomainGenome{}, fmt.Errorf("read genome: %w", err)
	}

	var genome DomainGenome
	if err := json.Unmarshal(data, &genome); err != nil {
		return DomainGenome{}, fmt.Errorf("decode genome: %w", err)
	}
	if err := genome.Validate(); err != nil {
		return DomainGenome{}, err
	}
	return genome, nil
}

func (s GenomeStore) Observe(observation GenomeObservation) (DomainGenome, string, error) {
	domain, err := domainFromURL(observation.URL)
	if err != nil {
		return DomainGenome{}, "", err
	}

	genome, err := s.Load(domain)
	if err != nil && !errors.Is(err, ErrGenomeNotFound) {
		return DomainGenome{}, "", err
	}
	if errors.Is(err, ErrGenomeNotFound) {
		genome = DomainGenome{Domain: domain}
	}

	now := s.now().UTC()
	genome.Domain = domain
	genome.SeenCount++
	genome.LastSeenAt = now
	genome.UpdatedAt = now
	if observation.ObservedLane > genome.ForceLane {
		genome.ForceLane = observation.ObservedLane
	}
	if strings.TrimSpace(observation.PreferredProfile) != "" {
		genome.PreferredProfile = observation.PreferredProfile
	}
	if pruningProfile := resolvePruningProfile(observation); pruningProfile != "" {
		genome.PruningProfile = pruningProfile
	}
	if observation.FetchMode == "render" || observation.RenderNeeded {
		genome.RenderNeeded = true
	}
	if strings.TrimSpace(observation.NoiseLevel) != "" {
		genome.NoiseLevel = observation.NoiseLevel
	}
	if strings.TrimSpace(observation.PageType) != "" {
		genome.LastPageType = observation.PageType
	}

	if err := genome.Validate(); err != nil {
		return DomainGenome{}, "", err
	}

	dir := filepath.Join(s.root, "genome")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return DomainGenome{}, "", fmt.Errorf("create genome dir: %w", err)
	}
	path := filepath.Join(dir, sanitizeID(domain)+".json")
	data, err := json.MarshalIndent(genome, "", "  ")
	if err != nil {
		return DomainGenome{}, "", fmt.Errorf("encode genome: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return DomainGenome{}, "", fmt.Errorf("write genome: %w", err)
	}
	return genome, path, nil
}

func (g DomainGenome) Validate() error {
	if strings.TrimSpace(g.Domain) == "" {
		return fmt.Errorf("domain is required")
	}
	if g.ForceLane < 0 || g.ForceLane > 4 {
		return fmt.Errorf("force_lane must be between 0 and 4")
	}
	if g.SeenCount < 0 {
		return fmt.Errorf("seen_count must be >= 0")
	}
	switch strings.TrimSpace(g.PruningProfile) {
	case "", "standard", "aggressive", "forum":
	default:
		return fmt.Errorf("pruning_profile must be one of %q, %q, or %q", "standard", "aggressive", "forum")
	}
	if g.SeenCount > 0 && g.LastSeenAt.IsZero() {
		return fmt.Errorf("last_seen_at is required when seen_count > 0")
	}
	if g.SeenCount > 0 && g.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at is required when seen_count > 0")
	}
	return nil
}

func domainFromURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	if strings.TrimSpace(parsed.Hostname()) == "" {
		return "", fmt.Errorf("url hostname is required")
	}
	return strings.ToLower(parsed.Hostname()), nil
}

func resolvePruningProfile(observation GenomeObservation) string {
	if strings.TrimSpace(observation.PruningProfile) != "" {
		return observation.PruningProfile
	}
	switch {
	case strings.EqualFold(observation.PageType, "forum"):
		return "forum"
	case strings.EqualFold(observation.NoiseLevel, "high"):
		return "aggressive"
	default:
		return "standard"
	}
}
