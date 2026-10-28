package discovery

import "testing"

func TestProviderUsagePreservesMixedSourceProvenance(t *testing.T) {
	for _, tc := range []struct {
		provider string
		want     ProviderUsage
	}{
		{" , , ", ProviderUsage{}},
		{"local_candidates", ProviderUsage{}},
		{" discovery_memory_sqlite ", ProviderUsage{LocalMemory: true}},
		{"discovery_memory_same_site", ProviderUsage{LocalMemory: true, SameSiteRecovery: true}},
		{"public_provider", ProviderUsage{PublicBootstrap: true}},
		{"local_same_site, public_provider", ProviderUsage{PublicBootstrap: true, SameSiteRecovery: true}},
		{" discovery_memory_sqlite, local_candidates, public_provider ", ProviderUsage{PublicBootstrap: true, LocalMemory: true}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			if got := ClassifyProviderUsage(tc.provider); got != tc.want {
				t.Fatalf("provider usage: got %+v want %+v", got, tc.want)
			}
		})
	}
}
