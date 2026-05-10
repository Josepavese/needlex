package providerfusion

import (
	"sort"
	"strconv"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

const (
	ReasonSemanticQuorum    = "semantic_quorum_provider_fusion"
	ReasonProviderConsensus = "semantic_provider_consensus"
)

type clusterEvidence struct {
	Members   []int
	Providers map[string]struct{}
}

func Apply(candidates []discoverycore.Candidate) []discoverycore.Candidate {
	if len(candidates) < 2 {
		return candidates
	}
	clusters := semanticClusters(candidates)
	out := append([]discoverycore.Candidate{}, candidates...)
	for _, cluster := range clusters {
		providerCount := len(cluster.Providers)
		if len(cluster.Members) < 2 || providerCount < 2 {
			continue
		}
		boost := min(0.16, 0.05*float64(providerCount)+0.025*float64(len(cluster.Members)-1))
		for _, idx := range cluster.Members {
			out[idx].Score += boost
			out[idx].Reason = discoverycore.AppendUniqueReason(out[idx].Reason, ReasonSemanticQuorum)
			out[idx].Metadata = discoverycore.MergeMetadata(out[idx].Metadata, map[string]string{
				"semantic_quorum_provider_count": strconv.Itoa(providerCount),
				"semantic_quorum_member_count":   strconv.Itoa(len(cluster.Members)),
				"semantic_quorum_boost":          formatFloat(boost),
			})
		}
	}
	applyProviderConsensus(out)
	discoverycore.SortCandidates(out)
	return out
}

func AnnotateProvider(candidates []discoverycore.Candidate, provider string) []discoverycore.Candidate {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return candidates
	}
	out := append([]discoverycore.Candidate{}, candidates...)
	for i := range out {
		out[i].Metadata = discoverycore.MergeMetadata(out[i].Metadata, map[string]string{
			"provider_observations": provider,
		})
	}
	return out
}

func semanticClusters(candidates []discoverycore.Candidate) map[string]clusterEvidence {
	clusters := map[string]clusterEvidence{}
	for idx, candidate := range candidates {
		key := strings.TrimSpace(candidate.Metadata["cluster_id"])
		if key == "" {
			continue
		}
		cluster := clusters[key]
		cluster.Members = append(cluster.Members, idx)
		if cluster.Providers == nil {
			cluster.Providers = map[string]struct{}{}
		}
		for _, provider := range splitProviders(candidate.Metadata["provider_observations"]) {
			cluster.Providers[provider] = struct{}{}
		}
		clusters[key] = cluster
	}
	for key, cluster := range clusters {
		if len(cluster.Providers) < 2 || len(cluster.Members) < 2 {
			delete(clusters, key)
		}
	}
	return clusters
}

func applyProviderConsensus(candidates []discoverycore.Candidate) {
	families := providerFamilies(candidates)
	for i := range candidates {
		providers := splitProviders(candidates[i].Metadata["provider_observations"])
		family, _ := candidateFamily(candidates[i].URL)
		familyProviders := families[family]
		providerCount := max(len(providers), len(familyProviders))
		if providerCount < 2 {
			continue
		}
		boost := min(0.14, 0.045*float64(providerCount))
		if len(providers) >= 2 {
			boost += 0.04
		}
		boost = min(boost, 0.18)
		candidates[i].Score += boost
		candidates[i].Reason = discoverycore.AppendUniqueReason(candidates[i].Reason, ReasonProviderConsensus)
		candidates[i].Metadata = discoverycore.MergeMetadata(candidates[i].Metadata, map[string]string{
			"semantic_provider_consensus_count": strconv.Itoa(providerCount),
			"semantic_provider_consensus_boost": formatFloat(boost),
		})
	}
}

func providerFamilies(candidates []discoverycore.Candidate) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for _, candidate := range candidates {
		family, ok := candidateFamily(candidate.URL)
		if !ok {
			continue
		}
		if out[family] == nil {
			out[family] = map[string]struct{}{}
		}
		for _, provider := range splitProviders(candidate.Metadata["provider_observations"]) {
			out[family][provider] = struct{}{}
		}
	}
	return out
}

func candidateFamily(rawURL string) (string, bool) {
	if family, err := discoverycore.RegistrableDomain(rawURL); err == nil && strings.TrimSpace(family) != "" {
		return strings.TrimSpace(family), true
	}
	if host, ok := discoverycore.Hostname(rawURL); ok && strings.TrimSpace(host) != "" {
		return strings.TrimSpace(host), true
	}
	return "", false
}

func splitProviders(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		provider := strings.TrimSpace(part)
		if provider == "" {
			continue
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		out = append(out, provider)
	}
	sort.Strings(out)
	return out
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
