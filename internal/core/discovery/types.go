package discovery

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/publicsuffix"
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

const (
	ResourceClassHTMLLike     = "html_like"
	ResourceClassDocumentFile = "document_file"
	ResourceClassStructured   = "structured_data"
	ResourceClassMediaAsset   = "media_asset"
	ResourceClassArchiveFile  = "archive_file"
	ResourceClassUnknown      = "unknown"
)

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
		metadata := map[string]string{"resource_class": ResourceClass(link.URL)}
		out = append(out, Candidate{
			URL:      link.URL,
			Label:    strings.TrimSpace(link.Label),
			Score:    score,
			Reason:   reason,
			Metadata: metadata,
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
	if pathBoost := urlStructureBoost(rawURL); pathBoost != 0 {
		score += pathBoost
		switch {
		case pathBoost > 0:
			reasons = append(reasons, "structure_hint")
		default:
			reasons = append(reasons, "structure_penalty")
		}
	}
	if classBoost := resourceClassBoost(rawURL); classBoost != 0 {
		score += classBoost
		if classBoost > 0 {
			reasons = append(reasons, "resource_class_hint")
		} else {
			reasons = append(reasons, "resource_class_penalty")
		}
	}
	if host, ok := Hostname(rawURL); ok && slices.Contains(domainHints, host) {
		score += 1.10
		reasons = append(reasons, "domain_hint_match")
	}
	if hostBoost := hostGoalCoherenceBoost(goal, rawURL); hostBoost > 0 {
		score += hostBoost
		reasons = append(reasons, "host_goal_coherence")
	}
	if urlBoost := urlGoalCoherenceBoost(goal, rawURL); urlBoost > 0 {
		score += urlBoost
		reasons = append(reasons, "url_goal_coherence")
	}
	if compactness := HostCompactnessBoost(rawURL); compactness != 0 {
		score += compactness
		if compactness > 0 {
			reasons = append(reasons, "host_compactness")
		}
	}
	if labelBoost := goalLabelAlignmentBoost(goal, label); labelBoost > 0 {
		score += labelBoost
		reasons = append(reasons, "goal_label_alignment")
	}

	return score, reasons
}

func ResourceClass(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ResourceClassUnknown
	}
	ext := strings.ToLower(strings.TrimSpace(path.Ext(parsed.Path)))
	switch ext {
	case "", ".html", ".htm", ".xhtml":
		return ResourceClassHTMLLike
	case ".pdf", ".txt", ".md":
		return ResourceClassDocumentFile
	case ".json", ".xml", ".rss", ".atom":
		return ResourceClassStructured
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".mp4", ".mp3":
		return ResourceClassMediaAsset
	case ".zip", ".gz", ".tgz":
		return ResourceClassArchiveFile
	default:
		return ResourceClassUnknown
	}
}

func ProbableNonHTMLURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	ext := strings.ToLower(strings.TrimSpace(path.Ext(parsed.Path)))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".pdf", ".xml", ".rss", ".atom", ".zip", ".gz", ".tgz", ".mp4", ".mp3":
		return true
	default:
		return false
	}
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

func HostTokenText(rawURL string) string {
	host, ok := Hostname(rawURL)
	if !ok {
		return rawURL
	}
	tokens := strings.FieldsFunc(host, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r))
	})
	if len(tokens) == 0 {
		return host
	}
	return strings.Join(tokens, " ")
}

func hostGoalCoherenceBoost(goal, rawURL string) float64 {
	return tokenOverlapBoost(goal, HostTokenText(rawURL))
}

func urlGoalCoherenceBoost(goal, rawURL string) float64 {
	return tokenOverlapBoost(goal, URLTokenText(rawURL)) * 0.55
}

func HostTitleCoherenceBoost(title, rawURL string) float64 {
	return tokenOverlapBoost(title, HostTokenText(rawURL))
}

func HostCompactnessBoost(rawURL string) float64 {
	host, ok := Hostname(rawURL)
	if !ok {
		return 0
	}
	registrable, err := RegistrableDomain(rawURL)
	if err != nil {
		return 0
	}
	hostLabels := strings.Split(host, ".")
	baseLabels := strings.Split(registrable, ".")
	extra := len(hostLabels) - len(baseLabels)
	switch {
	case extra <= 0:
		return 0.20
	case extra == 1:
		return 0.02
	default:
		return -0.06
	}
}

func RegistrableDomain(rawURL string) (string, error) {
	host, ok := Hostname(rawURL)
	if !ok {
		return "", fmt.Errorf("missing hostname")
	}
	return publicsuffix.EffectiveTLDPlusOne(host)
}

func URLPathDepth(rawURL string) int {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return 0
	}
	trimmedPath := strings.Trim(parsed.EscapedPath(), "/")
	if trimmedPath == "" {
		return 0
	}
	return len(strings.FieldsFunc(trimmedPath, func(r rune) bool { return r == '/' }))
}

func tokenOverlapBoost(left, right string) float64 {
	overlap := tokenOverlapRatio(left, right)
	switch {
	case overlap >= 0.50:
		return 0.65
	case overlap >= 0.34:
		return 0.40
	case overlap > 0:
		return 0.18
	default:
		return 0
	}
}

func tokenOverlapRatio(left, right string) float64 {
	leftTokens := textTokens(normalizeText(left))
	rightTokens := textTokens(normalizeText(right))
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}
	leftSet := make(map[string]struct{}, len(leftTokens))
	for _, token := range leftTokens {
		leftSet[token] = struct{}{}
	}
	rightSet := make(map[string]struct{}, len(rightTokens))
	for _, token := range rightTokens {
		rightSet[token] = struct{}{}
	}
	shared := 0
	for token := range leftSet {
		if _, ok := rightSet[token]; ok {
			shared++
		}
	}
	if shared == 0 {
		return 0
	}
	return float64(shared) / float64(max(len(leftSet), len(rightSet)))
}

func urlStructureBoost(rawURL string) float64 {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return 0
	}
	fragmentPenalty := 0.0
	if strings.TrimSpace(parsed.Fragment) != "" {
		fragmentPenalty = -0.10
	}
	trimmedPath := strings.Trim(parsed.EscapedPath(), "/")
	if trimmedPath == "" {
		return 0.22 + fragmentPenalty
	}
	segments := strings.FieldsFunc(trimmedPath, func(r rune) bool { return r == '/' })
	depth := len(segments)
	score := 0.0
	switch {
	case depth == 1:
		score += 0.16
	case depth == 2:
		score += 0.08
	case depth == 3:
		score -= 0.04
	case depth >= 4:
		score -= 0.20
	}
	if parsed.RawQuery == "" {
		score += 0.03
	}
	score += fragmentPenalty
	last := strings.ToLower(strings.TrimSpace(segments[len(segments)-1]))
	switch {
	case strings.HasPrefix(last, "class-"):
		score -= 0.10
	case strings.HasPrefix(last, "tag-"):
		score -= 0.10
	case strings.HasPrefix(last, "category-"):
		score -= 0.10
	}
	for _, segment := range segments {
		segment = strings.ToLower(strings.TrimSpace(segment))
		if _, err := strconv.Atoi(segment); err == nil {
			score -= 0.08
			continue
		}
		if len(segment) >= 18 && strings.Count(segment, "-") >= 2 {
			score -= 0.08
		}
		if len(segment) >= 24 && opaqueAlnumSegment(segment) {
			score -= 0.12
		}
		if strings.Contains(segment, ".html") || strings.Contains(segment, ".htm") {
			score -= 0.04
		}
	}
	return score
}

func resourceClassBoost(rawURL string) float64 {
	switch ResourceClass(rawURL) {
	case ResourceClassHTMLLike:
		return 0.12
	case ResourceClassDocumentFile:
		return 0.02
	case ResourceClassStructured:
		return -0.28
	case ResourceClassMediaAsset:
		return -0.18
	case ResourceClassArchiveFile:
		return -0.14
	default:
		return 0
	}
}

func opaqueAlnumSegment(segment string) bool {
	hasLetter := false
	hasDigit := false
	for _, r := range segment {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		case r == '-' || r == '_' || r == '.':
			continue
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}
