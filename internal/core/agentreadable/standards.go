package agentreadable

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var absoluteURLPattern = regexp.MustCompile(`https?://[^\s"'<>)]+`)

type RobotsPolicy struct {
	BaseURL  string
	Present  bool
	Sitemaps []string
	groups   []robotsGroup
}

type robotsGroup struct {
	agents []string
	rules  []robotsRule
}

type robotsRule struct {
	allow   bool
	pattern string
}

func RootRobotsURL(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.Path = "/robots.txt"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func ConventionalSitemapURLs(baseURL string) []string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}
	out := []string{}
	for _, candidatePath := range []string{"/sitemap.xml", "/sitemap_index.xml"} {
		next := *parsed
		next.Path = candidatePath
		next.RawQuery = ""
		next.Fragment = ""
		out = append(out, next.String())
	}
	return out
}

func SitemapURLsFromRobots(baseURL, robots string) []string {
	return ParseRobots(baseURL, robots).Sitemaps
}

func ParseRobots(baseURL, robots string) RobotsPolicy {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return RobotsPolicy{}
	}
	policy := RobotsPolicy{BaseURL: base.String(), Present: strings.TrimSpace(robots) != ""}
	var current robotsGroup
	flush := func() {
		if len(current.agents) > 0 || len(current.rules) > 0 {
			policy.groups = append(policy.groups, current)
		}
		current = robotsGroup{}
	}
	for _, line := range strings.Split(strings.ReplaceAll(robots, "\r\n", "\n"), "\n") {
		if hash := strings.Index(line, "#"); hash >= 0 {
			line = line[:hash]
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			if strings.TrimSpace(line) == "" {
				flush()
			}
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if len(current.rules) > 0 {
				flush()
			}
			if value != "" {
				current.agents = append(current.agents, strings.ToLower(value))
			}
		case "allow", "disallow":
			if len(current.agents) == 0 {
				continue
			}
			current.rules = append(current.rules, robotsRule{allow: key == "allow", pattern: value})
		case "sitemap":
			resolved := resolveURL(base, value)
			if resolved == "" || !sameOrigin(base.String(), resolved) {
				continue
			}
			policy.Sitemaps = append(policy.Sitemaps, resolved)
		}
	}
	flush()
	policy.Sitemaps = uniqueStrings(policy.Sitemaps)
	return policy
}

func (p RobotsPolicy) Allows(userAgent, rawURL string) bool {
	if !p.Present {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	if !sameOrigin(p.BaseURL, parsed.String()) {
		return false
	}
	pathQuery := parsed.EscapedPath()
	if pathQuery == "" {
		pathQuery = "/"
	}
	if parsed.RawQuery != "" {
		pathQuery += "?" + parsed.RawQuery
	}
	group, ok := p.matchingGroup(userAgent)
	if !ok {
		return true
	}
	bestLength := -1
	bestAllow := true
	for _, rule := range group.rules {
		pattern := strings.TrimSpace(rule.pattern)
		if pattern == "" {
			if !rule.allow {
				continue
			}
			pattern = "/"
		}
		if !robotsPatternMatches(pattern, pathQuery) {
			continue
		}
		length := len(pattern)
		if length > bestLength || (length == bestLength && rule.allow) {
			bestLength = length
			bestAllow = rule.allow
		}
	}
	return bestAllow
}

func (p RobotsPolicy) matchingGroup(userAgent string) (robotsGroup, bool) {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		ua = "needle-x"
	}
	bestLen := -1
	var best robotsGroup
	for _, group := range p.groups {
		for _, agent := range group.agents {
			agent = strings.ToLower(strings.TrimSpace(agent))
			if agent == "" {
				continue
			}
			matches := agent == "*" || strings.Contains(ua, agent)
			if matches && len(agent) > bestLen {
				best = group
				bestLen = len(agent)
			}
		}
	}
	return best, bestLen >= 0
}

func robotsPatternMatches(pattern, pathQuery string) bool {
	if pattern == "" {
		return false
	}
	if !strings.ContainsAny(pattern, "*$") {
		return strings.HasPrefix(pathQuery, pattern)
	}
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	position := 0
	for index, part := range parts {
		if part == "" {
			continue
		}
		found := strings.Index(pathQuery[position:], part)
		if found < 0 {
			return false
		}
		if index == 0 && !strings.HasPrefix(pathQuery, part) {
			return false
		}
		position += found + len(part)
	}
	if anchored {
		last := parts[len(parts)-1]
		return strings.HasSuffix(pathQuery, last)
	}
	return true
}

func CandidatesFromSitemap(targetURL, sitemapURL, sitemapBody string, maxCandidates int) []Candidate {
	base, err := url.Parse(strings.TrimSpace(sitemapURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil
	}
	target, _ := url.Parse(strings.TrimSpace(targetURL))
	locations := sitemapLocations(sitemapBody)
	out := make([]Candidate, 0, min(len(locations), max(1, maxCandidates)))
	for _, rawLocation := range locations {
		resolved := resolveURL(base, rawLocation)
		if resolved == "" || !sameOrigin(base.String(), resolved) {
			continue
		}
		if candidate, ok := candidateFromProtocolURL(resolved, "sitemap"); ok {
			candidate.Priority += sitemapPathPriority(target, resolved)
			out = append(out, candidate)
		}
	}
	return NormalizeCandidates(out, maxCandidates)
}

func CandidatesFromAPICatalog(catalogURL, body string, maxCandidates int) []Candidate {
	base, err := url.Parse(strings.TrimSpace(catalogURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil
	}
	out := []Candidate{}
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err == nil {
		collectCatalogCandidates(base, decoded, "", &out)
	}
	var yamlDecoded any
	if err := yaml.Unmarshal([]byte(body), &yamlDecoded); err == nil {
		collectCatalogCandidates(base, normalizeYAMLValue(yamlDecoded), "", &out)
	}
	for _, raw := range absoluteURLPattern.FindAllString(body, -1) {
		if candidate, ok := candidateFromProtocolURL(raw, "api_catalog"); ok && sameOrigin(base.String(), candidate.URL) {
			out = append(out, candidate)
		}
	}
	return NormalizeCandidates(out, maxCandidates)
}
