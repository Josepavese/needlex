package discovery

import "strings"

// ProviderUsage describes provenance, not relevance. Provider records may
// contain a comma-separated chain of local and public discovery sources.
type ProviderUsage struct {
	PublicBootstrap, LocalMemory, SameSiteRecovery bool
}

func ClassifyProviderUsage(provider string) ProviderUsage {
	usage := ProviderUsage{
		LocalMemory:      strings.Contains(strings.TrimSpace(provider), "discovery_memory"),
		SameSiteRecovery: strings.Contains(provider, "same_site"),
	}
	for _, part := range strings.Split(provider, ",") {
		part = strings.TrimSpace(part)
		if part != "" && !strings.HasPrefix(part, "local_") && !strings.HasPrefix(part, "discovery_memory") {
			usage.PublicBootstrap = true
			break
		}
	}
	return usage
}
