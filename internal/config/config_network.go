package config

import "github.com/josepavese/needlex/internal/platform"

// EgressPolicy resolves the configured network policy for acquisition transports
// and rendering. Validation has already rejected unknown values.
func (c Config) EgressPolicy() platform.NetworkPolicy {
	policy, err := platform.ParseNetworkPolicy(c.Network.Policy)
	if err != nil {
		return platform.NetworkPolicyAny
	}
	return policy
}
