package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrCandidatesNotFound = errors.New("candidates not found")

type CandidateRecord struct {
	URL       string    `json:"url"`
	Title     string    `json:"title,omitempty"`
	Host      string    `json:"host"`
	SeenCount int       `json:"seen_count"`
	LastSeen  time.Time `json:"last_seen"`
	Sources   []string  `json:"sources,omitempty"`
}

type CandidateMatch struct {
	URL    string   `json:"url"`
	Title  string   `json:"title,omitempty"`
	Score  float64  `json:"score"`
	Reason []string `json:"reason,omitempty"`
}

type CandidateObservation struct {
	URL    string
	Title  string
	Source string
}

type CandidateStore struct {
	root string
	now  func() time.Time
}

func NewCandidateStore(root string) CandidateStore {
	if strings.TrimSpace(root) == "" {
		root = ".needlex"
	}
	return CandidateStore{
		root: root,
		now:  time.Now,
	}
}

func (s CandidateStore) Observe(observation CandidateObservation) (CandidateRecord, string, error) {
	cleanURL := strings.TrimSpace(observation.URL)
	if cleanURL == "" {
		return CandidateRecord{}, "", fmt.Errorf("observation.url must not be empty")
	}
	host, err := hostFromURL(cleanURL)
	if err != nil {
		return CandidateRecord{}, "", err
	}

	records, _ := s.loadAll()
	now := s.now().UTC()
	updated := CandidateRecord{}
	found := false
	for i := range records {
		if records[i].URL != cleanURL {
			continue
		}
		records[i].SeenCount++
		records[i].LastSeen = now
		if strings.TrimSpace(observation.Title) != "" {
			records[i].Title = strings.TrimSpace(observation.Title)
		}
		records[i].Sources = appendUnique(records[i].Sources, strings.TrimSpace(observation.Source))
		updated = records[i]
		found = true
		break
	}
	if !found {
		updated = CandidateRecord{
			URL:       cleanURL,
			Title:     strings.TrimSpace(observation.Title),
			Host:      host,
			SeenCount: 1,
			LastSeen:  now,
			Sources:   appendUnique(nil, strings.TrimSpace(observation.Source)),
		}
		records = append(records, updated)
	}

	path, err := s.saveAll(records)
	if err != nil {
		return CandidateRecord{}, "", err
	}
	return updated, path, nil
}

func (s CandidateStore) Search(goal string, limit int) ([]CandidateMatch, error) {
	records, err := s.loadAll()
	if err != nil {
		if errors.Is(err, ErrCandidatesNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if limit <= 0 {
		limit = 5
	}
	goalTokens := candidateTokens(goal)
	if len(goalTokens) == 0 {
		return nil, nil
	}

	matches := make([]CandidateMatch, 0, len(records))
	for _, record := range records {
		score, reasons := scoreCandidate(goalTokens, record)
		if score <= 0 {
			continue
		}
		matches = append(matches, CandidateMatch{
			URL:    record.URL,
			Title:  record.Title,
			Score:  score,
			Reason: reasons,
		})
	}
	slices.SortStableFunc(matches, func(left, right CandidateMatch) int {
		switch {
		case left.Score > right.Score:
			return -1
		case left.Score < right.Score:
			return 1
		case left.URL < right.URL:
			return -1
		case left.URL > right.URL:
			return 1
		default:
			return 0
		}
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func (s CandidateStore) loadAll() ([]CandidateRecord, error) {
	path := filepath.Join(s.root, "candidates", "index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrCandidatesNotFound
		}
		return nil, fmt.Errorf("read candidates: %w", err)
	}
	var records []CandidateRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode candidates: %w", err)
	}
	return records, nil
}

func (s CandidateStore) saveAll(records []CandidateRecord) (string, error) {
	dir := filepath.Join(s.root, "candidates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create candidates dir: %w", err)
	}
	path := filepath.Join(dir, "index.json")
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode candidates: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write candidates: %w", err)
	}
	return path, nil
}

func scoreCandidate(goalTokens []string, record CandidateRecord) (float64, []string) {
	recordTokens := candidateTokens(record.Title + " " + record.URL)
	if len(recordTokens) == 0 {
		return 0, nil
	}
	matches := overlapCount(goalTokens, recordTokens)
	if matches == 0 {
		return 0, nil
	}
	score := float64(matches) + float64(min(record.SeenCount, 5))*0.15
	reasons := []string{"goal_token_overlap"}
	if record.SeenCount > 1 {
		reasons = append(reasons, "local_history")
	}
	return score, reasons
}

func candidateTokens(text string) []string {
	raw := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		if len(item) < 3 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func overlapCount(left, right []string) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(right))
	for _, token := range right {
		set[token] = struct{}{}
	}
	count := 0
	for _, token := range left {
		if _, ok := set[token]; ok {
			count++
		}
	}
	return count
}

func appendUnique(existing []string, values ...string) []string {
	seen := map[string]struct{}{}
	out := append([]string{}, existing...)
	for _, item := range out {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func hostFromURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse url: %w", err)
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" {
		return "", fmt.Errorf("url hostname is required")
	}
	return host, nil
}
