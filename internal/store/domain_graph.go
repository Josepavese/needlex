package store

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
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
	documents platform.JSONDocuments
	now       func() time.Time
}

func NewDomainGraphStore(root string) DomainGraphStore {
	return DomainGraphStore{
		documents: platform.NewJSONDocuments(platform.NewStateLayout(root).DomainGraphDir, "domain graph", "domain graph"),
		now:       time.Now,
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

	graph, err := s.loadAll()
	if err != nil && !errors.Is(err, ErrDomainGraphNotFound) {
		return DomainTransition{}, "", err
	}
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
	return buildDomainMatches(graph, seeds, limit), nil
}

type domainScore struct {
	value  float64
	reason []string
}

func buildDomainMatches(graph []DomainTransition, seeds []string, limit int) []DomainMatch {
	acc := accumulateDomainScores(graph, seeds)
	matches := collectDomainMatches(acc)
	sortDomainMatches(matches)
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func accumulateDomainScores(graph []DomainTransition, seeds []string) map[string]domainScore {
	acc := map[string]domainScore{}
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
	return acc
}

func collectDomainMatches(acc map[string]domainScore) []DomainMatch {
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
	return matches
}

func sortDomainMatches(matches []DomainMatch) {
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
}

func (s DomainGraphStore) loadAll() ([]DomainTransition, error) {
	var graph []DomainTransition
	if err := s.documents.Load("index", &graph); err != nil {
		if errors.Is(err, platform.ErrDocumentNotFound) {
			return nil, ErrDomainGraphNotFound
		}
		return nil, err
	}
	return graph, nil
}

func (s DomainGraphStore) saveAll(graph []DomainTransition) (string, error) {
	return s.documents.Save("index", graph)
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
