package discovery

import (
	"net/url"
	"path"
	"slices"
	"sort"
	"strings"
	"unicode"
)

type LinkCandidate struct {
	URL   string
	Label string
}

type Candidate struct {
	URL      string            `json:"url"`
	Label    string            `json:"label,omitempty"`
	Score    float64           `json:"score"`
	Reason   []string          `json:"reason,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func ScoreCandidates(goal, seedURL, seedLabel string, links []LinkCandidate, domainHints []string) []Candidate {
	domainHints = NormalizeDomainHints(domainHints)
	out := make([]Candidate, 0, len(links)+1)
	seen := map[string]struct{}{}

	if strings.TrimSpace(seedURL) != "" {
		seedScore, seedReason := score(goal, seedURL, seedLabel, seedURL, true, domainHints)
		out = append(out, Candidate{
			URL:    seedURL,
			Label:  strings.TrimSpace(seedLabel),
			Score:  seedScore,
			Reason: seedReason,
		})
		seen[seedURL] = struct{}{}
	}

	for _, link := range links {
		if _, ok := seen[link.URL]; ok {
			continue
		}
		seen[link.URL] = struct{}{}
		score, reason := score(goal, seedURL, link.Label, link.URL, false, domainHints)
		out = append(out, Candidate{
			URL:    link.URL,
			Label:  strings.TrimSpace(link.Label),
			Score:  score,
			Reason: reason,
		})
	}

	SortCandidates(out)
	return out
}

func ScoreURL(goal, rawURL, label string, isSeed bool, domainHints []string) (float64, []string) {
	return score(goal, "", label, rawURL, isSeed, NormalizeDomainHints(domainHints))
}

func SortCandidates(candidates []Candidate) {
	slices.SortStableFunc(candidates, func(left, right Candidate) int {
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
}

func AppendUniqueReason(existing []string, incoming ...string) []string {
	seen := make(map[string]struct{}, len(existing))
	out := append([]string{}, existing...)
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range incoming {
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

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func NormalizeDomainHints(hints []string) []string {
	out := make([]string, 0, len(hints))
	seen := map[string]struct{}{}
	for _, hint := range hints {
		host := strings.TrimSpace(strings.ToLower(hint))
		if host == "" {
			continue
		}
		if parsed, err := url.Parse(host); err == nil && strings.TrimSpace(parsed.Hostname()) != "" {
			host = strings.ToLower(strings.TrimSpace(parsed.Hostname()))
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	return out
}

func Hostname(rawURL string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	if host == "" {
		return "", false
	}
	return host, true
}

func score(goal, seedURL, label, rawURL string, isSeed bool, domainHints []string) (float64, []string) {
	reasons := []string{}
	score := 0.0

	if isSeed {
		score += 0.35
		reasons = append(reasons, "seed_fallback")
	}
	if strings.Contains(strings.ToLower(rawURL), "docs") || strings.Contains(strings.ToLower(rawURL), "guide") {
		score += 0.20
		reasons = append(reasons, "path_hint")
	}
	if host, ok := Hostname(rawURL); ok && slices.Contains(domainHints, host) {
		score += 1.10
		reasons = append(reasons, "domain_hint_match")
	}
	if labelBoost := goalLabelAlignmentBoost(goal, label); labelBoost > 0 {
		score += labelBoost
		reasons = append(reasons, "goal_label_alignment")
	}

	return score, reasons
}

func goalLabelAlignmentBoost(goal, label string) float64 {
	goal = normalizeText(goal)
	label = normalizeText(label)
	if goal == "" || label == "" {
		return 0
	}
	if goal == label {
		return 0.70
	}
	if strings.Contains(label, goal) || strings.Contains(goal, label) {
		return 0.45
	}

	goalTokens := textTokens(goal)
	labelTokens := textTokens(label)
	if len(goalTokens) == 0 || len(labelTokens) == 0 {
		return 0
	}
	goalSet := make(map[string]struct{}, len(goalTokens))
	for _, token := range goalTokens {
		goalSet[token] = struct{}{}
	}
	matched := 0
	for _, token := range labelTokens {
		if _, ok := goalSet[token]; ok {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	coverage := float64(matched) / float64(max(len(goalTokens), len(labelTokens)))
	switch {
	case matched >= 2 && coverage >= 0.5:
		return 0.35
	case matched >= 1:
		return 0.18
	default:
		return 0
	}
}

func normalizeText(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			lastSpace = false
		case !lastSpace:
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func textTokens(value string) []string {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		out = append(out, part)
	}
	sort.Strings(out)
	return out
}

func JoinNonEmpty(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return strings.Join(parts, " ")
}

func URLTokenText(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return strings.Join([]string{parsed.Hostname(), parsed.Path, path.Base(parsed.Path)}, " ")
}
