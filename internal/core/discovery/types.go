package discovery

import (
	"fmt"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/publicsuffix"
)

type LinkCandidate struct {
	URL     string
	Label   string
	Context string
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
	ResourceClassTextAsset    = "text_asset"
	ResourceClassMediaAsset   = "media_asset"
	ResourceClassArchiveFile  = "archive_file"
	ResourceClassUnknown      = "unknown"
)

func ScoreStructuralCandidates(seedURL, seedLabel string, links []LinkCandidate, domainHints []string) []Candidate {
	domainHints = NormalizeDomainHints(domainHints)
	out := make([]Candidate, 0, len(links)+1)
	seen := map[string]struct{}{}

	if strings.TrimSpace(seedURL) != "" {
		seedScore, seedReason := structuralPriorScore(seedURL, true, domainHints)
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
		score, reason := structuralPriorScore(link.URL, false, domainHints)
		metadata := map[string]string{"resource_class": ResourceClass(link.URL)}
		if context := strings.TrimSpace(link.Context); context != "" {
			metadata["source_context"] = compactDiscoveryText(context, 900)
		}
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

func ScoreStructuralURL(rawURL string, isSeed bool, domainHints []string) (float64, []string) {
	return structuralPriorScore(rawURL, isSeed, NormalizeDomainHints(domainHints))
}

func ApplySameSiteContextPrior(seedURL string, candidates []Candidate) []Candidate {
	if strings.TrimSpace(seedURL) == "" || len(candidates) < 2 {
		return candidates
	}
	seed, err := url.Parse(strings.TrimSpace(seedURL))
	if err != nil {
		return candidates
	}
	seedHost := strings.TrimSpace(strings.ToLower(seed.Hostname()))
	if seedHost == "" || !hasSameSiteAlternative(seedHost, seedURL, candidates) {
		return candidates
	}

	seedDepth := URLPathDepth(seedURL)
	seedScope := sameSiteContextScope(seed)
	stats := buildSameSiteContextStats(seedHost, seedURL, candidates)
	out := append([]Candidate{}, candidates...)
	for i := range out {
		boost, reasons := sameSiteContextBoost(seedHost, seedURL, seedDepth, seedScope, stats, out[i])
		if boost == 0 {
			continue
		}
		out[i].Score += boost
		out[i].Reason = AppendUniqueReason(out[i].Reason, reasons...)
		out[i].Metadata = MergeMetadata(out[i].Metadata, map[string]string{
			"same_site_context_prior": strconv.FormatFloat(boost, 'f', 2, 64),
		})
	}
	SortCandidates(out)
	return out
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

func structuralPriorScore(rawURL string, isSeed bool, domainHints []string) (float64, []string) {
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
		reasons = append(reasons, "domain_identity_match")
	}
	if compactness := HostCompactnessBoost(rawURL); compactness != 0 {
		score += compactness
		if compactness > 0 {
			reasons = append(reasons, "host_compactness")
		}
	}

	return score, reasons
}

func hasSameSiteAlternative(seedHost, seedURL string, candidates []Candidate) bool {
	for _, candidate := range candidates {
		if sameNormalizedDiscoveryURL(seedURL, candidate.URL) {
			continue
		}
		host, ok := Hostname(candidate.URL)
		if !ok || host != seedHost {
			continue
		}
		if ResourceClass(candidate.URL) == ResourceClassArchiveFile {
			continue
		}
		return true
	}
	return false
}

type sameSiteContextStats struct {
	DominantFirstSegment string
	DominantCount        int
}

func buildSameSiteContextStats(seedHost, seedURL string, candidates []Candidate) sameSiteContextStats {
	counts := map[string]int{}
	for _, candidate := range candidates {
		if sameNormalizedDiscoveryURL(seedURL, candidate.URL) {
			continue
		}
		host, ok := Hostname(candidate.URL)
		if !ok || host != seedHost {
			continue
		}
		segment := firstDiscoverySegment(candidate.URL)
		if segment == "" || numericDenseRouteSegment(segment) {
			continue
		}
		counts[segment]++
	}
	stats := sameSiteContextStats{}
	for segment, count := range counts {
		if count > stats.DominantCount {
			stats.DominantFirstSegment = segment
			stats.DominantCount = count
		}
	}
	if stats.DominantCount < 3 {
		return sameSiteContextStats{}
	}
	return stats
}

func sameSiteContextBoost(seedHost, seedURL string, seedDepth int, seedScope string, stats sameSiteContextStats, candidate Candidate) (float64, []string) {
	if sameNormalizedDiscoveryURL(seedURL, candidate.URL) {
		return -0.12, []string{"same_site_seed_context_fallback"}
	}
	host, ok := Hostname(candidate.URL)
	if !ok || host != seedHost {
		return 0, nil
	}
	parsed, err := url.Parse(strings.TrimSpace(candidate.URL))
	if err != nil {
		return 0, nil
	}

	candidatePath := normalizeDiscoveryPath(parsed)
	candidateDepth := URLPathDepth(candidate.URL)
	boost := 0.14
	reasons := []string{"same_site_context_prior"}

	appendContextBoost(&boost, &reasons, anchorContextBoost(candidate.Label))
	appendContextBoost(&boost, &reasons, resourceClassContextBoost(candidate.URL))
	appendContextBoost(&boost, &reasons, sameSiteRouteRelationBoost(seedDepth, candidateDepth, candidatePath, seedScope))
	appendContextBoost(&boost, &reasons, sameSiteScopeContinuityBoost(seedScope, candidatePath))
	appendContextBoost(&boost, &reasons, dominantPathFamilyBoost(seedDepth, stats, candidate.URL, candidatePath))
	appendContextBoost(&boost, &reasons, fragmentRouteBoost(parsed, candidatePath, seedScope))
	return boost, reasons
}

type contextBoost struct {
	Value   float64
	Reasons []string
}

func appendContextBoost(boost *float64, reasons *[]string, item contextBoost) {
	if item.Value == 0 && len(item.Reasons) == 0 {
		return
	}
	*boost += item.Value
	*reasons = append(*reasons, item.Reasons...)
}

func anchorContextBoost(label string) contextBoost {
	if strings.TrimSpace(label) == "" {
		return contextBoost{}
	}
	return contextBoost{Value: 0.06, Reasons: []string{"anchor_context_present"}}
}

func resourceClassContextBoost(rawURL string) contextBoost {
	switch ResourceClass(rawURL) {
	case ResourceClassHTMLLike:
		return contextBoost{Value: 0.04}
	case ResourceClassDocumentFile, ResourceClassStructured, ResourceClassTextAsset:
		return contextBoost{Value: 0.01}
	case ResourceClassMediaAsset:
		return contextBoost{Value: -0.06}
	case ResourceClassArchiveFile:
		return contextBoost{Value: -0.12}
	default:
		return contextBoost{}
	}
}

func sameSiteRouteRelationBoost(seedDepth, candidateDepth int, candidatePath, seedScope string) contextBoost {
	switch {
	case seedDepth == 0 && candidateDepth > 0:
		return contextBoost{Value: min(0.54, 0.18+float64(candidateDepth)*0.12), Reasons: []string{"same_site_specific_route"}}
	case candidateDepth > seedDepth:
		return contextBoost{Value: min(0.50, 0.22+float64(candidateDepth-seedDepth)*0.12), Reasons: []string{"same_site_deeper_route"}}
	case candidateDepth == seedDepth && sameDiscoveryParent(candidatePath, seedScope):
		return contextBoost{Value: 0.30, Reasons: []string{"same_site_sibling_route"}}
	case candidateDepth > 0 && !isDiscoveryAncestor(candidatePath, seedScope):
		return contextBoost{Value: 0.22, Reasons: []string{"same_site_peer_section"}}
	case candidateDepth == 0 && seedDepth > 0:
		return contextBoost{Value: -0.42, Reasons: []string{"same_site_scope_regression"}}
	case candidateDepth < seedDepth && isDiscoveryAncestor(candidatePath, seedScope):
		return contextBoost{Value: -0.28, Reasons: []string{"same_site_scope_regression"}}
	default:
		return contextBoost{}
	}
}

func sameSiteScopeContinuityBoost(seedScope, candidatePath string) contextBoost {
	if seedScope == "/" || !isDiscoveryDescendant(seedScope, candidatePath) {
		return contextBoost{}
	}
	return contextBoost{Value: 0.18, Reasons: []string{"same_site_scope_continuity"}}
}

func dominantPathFamilyBoost(seedDepth int, stats sameSiteContextStats, rawURL, candidatePath string) contextBoost {
	if seedDepth != 0 || stats.DominantFirstSegment == "" || firstDiscoverySegment(rawURL) != stats.DominantFirstSegment {
		return contextBoost{}
	}
	boost := contextBoost{Value: 0.16, Reasons: []string{"same_site_dominant_path_family"}}
	if simpleRouteRepresentative(candidatePath) {
		boost.Value += 0.34
		boost.Reasons = append(boost.Reasons, "same_site_family_representative")
	}
	return boost
}

func fragmentRouteBoost(parsed *url.URL, candidatePath, seedScope string) contextBoost {
	if strings.TrimSpace(parsed.Fragment) == "" || !sameDiscoveryParent(candidatePath, seedScope) {
		return contextBoost{}
	}
	return contextBoost{Value: -0.10, Reasons: []string{"fragment_route_penalty"}}
}

func sameSiteContextScope(parsed *url.URL) string {
	normalized := normalizeDiscoveryPath(parsed)
	if normalized == "/" {
		return "/"
	}
	base := strings.ToLower(path.Base(normalized))
	if path.Ext(base) != "" || strings.HasPrefix(base, "index.") {
		return parentDiscoveryPath(normalized)
	}
	return normalized
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
	case ".css", ".js", ".mjs", ".map":
		return ResourceClassTextAsset
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".mp4", ".mp3":
		return ResourceClassMediaAsset
	case ".zip", ".gz", ".tgz":
		return ResourceClassArchiveFile
	default:
		return ResourceClassUnknown
	}
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

func compactDiscoveryText(value string, maxRunes int) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	compact := strings.Join(fields, " ")
	if maxRunes <= 0 {
		return compact
	}
	runes := []rune(compact)
	if len(runes) <= maxRunes {
		return compact
	}
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func URLIdentityText(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return strings.Join([]string{parsed.Hostname(), parsed.Path, path.Base(parsed.Path)}, " ")
}

func HostIdentityText(rawURL string) string {
	host, ok := Hostname(rawURL)
	if !ok {
		return rawURL
	}
	hostParts := strings.FieldsFunc(host, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r))
	})
	if len(hostParts) == 0 {
		return host
	}
	return strings.Join(hostParts, " ")
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

func normalizeDiscoveryPath(parsed *url.URL) string {
	trimmed := strings.TrimSpace(parsed.EscapedPath())
	if trimmed == "" || trimmed == "/" {
		return "/"
	}
	normalized := path.Clean("/" + strings.Trim(trimmed, "/"))
	if normalized == "." || normalized == "" {
		return "/"
	}
	return normalized
}

func parentDiscoveryPath(rawPath string) string {
	parent := path.Dir(rawPath)
	if parent == "." || parent == "" {
		return "/"
	}
	return parent
}

func sameDiscoveryParent(candidatePath, scopePath string) bool {
	return parentDiscoveryPath(candidatePath) == scopePath
}

func isDiscoveryDescendant(scopePath, candidatePath string) bool {
	if scopePath == "/" {
		return candidatePath != "/"
	}
	return strings.HasPrefix(candidatePath, strings.TrimRight(scopePath, "/")+"/")
}

func isDiscoveryAncestor(candidatePath, scopePath string) bool {
	if candidatePath == "/" {
		return scopePath != "/"
	}
	return strings.HasPrefix(scopePath, strings.TrimRight(candidatePath, "/")+"/")
}

func sameNormalizedDiscoveryURL(leftURL, rightURL string) bool {
	left, errLeft := url.Parse(strings.TrimSpace(leftURL))
	right, errRight := url.Parse(strings.TrimSpace(rightURL))
	if errLeft != nil || errRight != nil {
		return false
	}
	return strings.EqualFold(left.Hostname(), right.Hostname()) &&
		normalizeDiscoveryPath(left) == normalizeDiscoveryPath(right)
}

func firstDiscoverySegment(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	trimmed := strings.Trim(normalizeDiscoveryPath(parsed), "/")
	if trimmed == "" {
		return ""
	}
	return strings.ToLower(strings.Split(trimmed, "/")[0])
}

func simpleRouteRepresentative(candidatePath string) bool {
	trimmed := strings.Trim(candidatePath, "/")
	if trimmed == "" {
		return false
	}
	segments := strings.Split(trimmed, "/")
	if len(segments) != 2 {
		return false
	}
	stem := routeSegmentStem(segments[len(segments)-1])
	if stem == "" || len([]rune(stem)) > 12 {
		return false
	}
	if strings.ContainsAny(stem, "-_") || numericDenseRouteSegment(stem) {
		return false
	}
	return true
}

func routeSegmentStem(segment string) string {
	segment = strings.ToLower(strings.TrimSpace(segment))
	ext := path.Ext(segment)
	if ext != "" {
		segment = strings.TrimSuffix(segment, ext)
	}
	return strings.Trim(segment, ".")
}

func numericDenseRouteSegment(segment string) bool {
	stem := routeSegmentStem(segment)
	if stem == "" {
		return false
	}
	digits := 0
	letters := 0
	for _, r := range stem {
		switch {
		case unicode.IsDigit(r):
			digits++
		case unicode.IsLetter(r):
			letters++
		}
	}
	return digits >= 2 && digits >= letters
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
	score := pathDepthStructureBoost(len(segments))
	if parsed.RawQuery == "" {
		score += 0.03
	}
	score += fragmentPenalty
	score += terminalRoutePenalty(segments[len(segments)-1])
	for _, segment := range segments {
		score += routeSegmentStructurePenalty(segment)
	}
	return score
}

func pathDepthStructureBoost(depth int) float64 {
	switch {
	case depth == 1:
		return 0.16
	case depth == 2:
		return 0.08
	case depth == 3:
		return -0.04
	case depth >= 4:
		return -0.20
	default:
		return 0
	}
}

func terminalRoutePenalty(segment string) float64 {
	last := strings.ToLower(strings.TrimSpace(segment))
	switch {
	case strings.HasPrefix(last, "class-"):
		return -0.10
	case strings.HasPrefix(last, "tag-"):
		return -0.10
	case strings.HasPrefix(last, "category-"):
		return -0.10
	default:
		return 0
	}
}

func routeSegmentStructurePenalty(segment string) float64 {
	segment = strings.ToLower(strings.TrimSpace(segment))
	if _, err := strconv.Atoi(segment); err == nil {
		return -0.08
	}
	score := 0.0
	if len(segment) >= 18 && strings.Count(segment, "-") >= 2 {
		score -= 0.08
	}
	if len(segment) >= 24 && opaqueAlnumSegment(segment) {
		score -= 0.12
	}
	if strings.Contains(segment, ".html") || strings.Contains(segment, ".htm") {
		score -= 0.04
	}
	if numericDenseRouteSegment(segment) {
		score -= 0.12
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
	case ResourceClassTextAsset:
		return -0.04
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
