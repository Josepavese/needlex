package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrDomainGraphNotFound = errors.New("domain graph not found")

type DomainTransition struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	Count    int       `json:"count"`
	LastSeen time.Time `json:"last_seen"`
	Sources  []string  `json:"sources,omitempty"`
}

type DomainMatch struct {
	Domain string   `json:"domain"`
	Score  float64  `json:"score"`
	Reason []string `json:"reason,omitempty"`
}

type DomainGraphStore struct {
	root string
	now  func() time.Time
}

func NewDomainGraphStore(root string) DomainGraphStore {
	if strings.TrimSpace(root) == "" {
		root = ".needlex"
	}
	return DomainGraphStore{
		root: root,
		now:  time.Now,
	}
}

func (s DomainGraphStore) Observe(fromURL, toURL, source string) (DomainTransition, string, error) {
	from, err := hostFromURL(strings.TrimSpace(fromURL))
	if err != nil {
		return DomainTransition{}, "", err
	}
	to, err := hostFromURL(strings.TrimSpace(toURL))
	if err != nil {
		return DomainTransition{}, "", err
	}
	if from == to {
		return DomainTransition{}, "", nil
	}

	graph, _ := s.loadAll()
	now := s.now().UTC()
	updated := DomainTransition{}
	found := false
	for i := range graph {
		if graph[i].From != from || graph[i].To != to {
			continue
		}
		graph[i].Count++
		graph[i].LastSeen = now
		graph[i].Sources = appendUnique(graph[i].Sources, source)
		updated = graph[i]
		found = true
		break
	}
	if !found {
		updated = DomainTransition{
			From:     from,
			To:       to,
			Count:    1,
			LastSeen: now,
			Sources:  appendUnique(nil, source),
		}
		graph = append(graph, updated)
	}
	path, err := s.saveAll(graph)
	if err != nil {
		return DomainTransition{}, "", err
	}
	return updated, path, nil
}

func (s DomainGraphStore) Expand(seedDomains []string, limit int) ([]DomainMatch, error) {
	graph, err := s.loadAll()
	if err != nil {
		if errors.Is(err, ErrDomainGraphNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if limit <= 0 {
		limit = 5
	}
	seeds := normalizeSeedDomains(seedDomains)
	if len(seeds) == 0 {
		return nil, nil
	}

	type score struct {
		value  float64
		reason []string
	}
	acc := map[string]score{}
	for _, edge := range graph {
		if edge.Count <= 0 {
			continue
		}
		fromSeed := slices.Contains(seeds, edge.From)
		toSeed := slices.Contains(seeds, edge.To)
		switch {
		case fromSeed && !toSeed:
			current := acc[edge.To]
			current.value += float64(edge.Count)
			current.reason = appendUnique(current.reason, "outbound_transition")
			acc[edge.To] = current
		case toSeed && !fromSeed:
			current := acc[edge.From]
			current.value += float64(edge.Count) * 0.5
			current.reason = appendUnique(current.reason, "inbound_transition")
			acc[edge.From] = current
		}
	}

	matches := make([]DomainMatch, 0, len(acc))
	for domain, item := range acc {
		if item.value <= 0 {
			continue
		}
		matches = append(matches, DomainMatch{
			Domain: domain,
			Score:  item.value,
			Reason: item.reason,
		})
	}
	slices.SortStableFunc(matches, func(left, right DomainMatch) int {
		switch {
		case left.Score > right.Score:
			return -1
		case left.Score < right.Score:
			return 1
		case left.Domain < right.Domain:
			return -1
		case left.Domain > right.Domain:
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

func (s DomainGraphStore) loadAll() ([]DomainTransition, error) {
	path := filepath.Join(s.root, "domain_graph", "index.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrDomainGraphNotFound
		}
		return nil, fmt.Errorf("read domain graph: %w", err)
	}
	var graph []DomainTransition
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, fmt.Errorf("decode domain graph: %w", err)
	}
	return graph, nil
}

func (s DomainGraphStore) saveAll(graph []DomainTransition) (string, error) {
	dir := filepath.Join(s.root, "domain_graph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create domain graph dir: %w", err)
	}
	path := filepath.Join(dir, "index.json")
	data, err := json.MarshalIndent(graph, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode domain graph: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write domain graph: %w", err)
	}
	return path, nil
}

func normalizeSeedDomains(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		domain := strings.TrimSpace(strings.ToLower(value))
		if domain == "" {
			continue
		}
		if host, err := hostFromURL(domain); err == nil && host != "" {
			domain = host
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out
}
