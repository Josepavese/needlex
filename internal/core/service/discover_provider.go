package service

import (
	"context"
	"sort"
	"strings"
	"time"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/store"
)

type discoveryProviderAdapter interface {
	Name() string
	BaseURL() string
	Bootstrap(context.Context, DiscoverWebRequest, string) ([]DiscoverCandidate, string, error)
}

type serviceDiscoveryProviderAdapter struct {
	svc     *Service
	baseURL string
}

func (a serviceDiscoveryProviderAdapter) Name() string {
	return discoverycore.ProviderName(a.baseURL)
}

func (a serviceDiscoveryProviderAdapter) BaseURL() string {
	return a.baseURL
}

func (a serviceDiscoveryProviderAdapter) Bootstrap(ctx context.Context, req DiscoverWebRequest, query string) ([]DiscoverCandidate, string, error) {
	return a.svc.discoverWebBootstrap(ctx, a.baseURL, req, query)
}

func (s *Service) discoveryProviderAdapters(baseURLs []string) []discoveryProviderAdapter {
	ordered := s.orderedDiscoveryProviders(baseURLs)
	out := make([]discoveryProviderAdapter, 0, len(ordered))
	for _, baseURL := range ordered {
		out = append(out, serviceDiscoveryProviderAdapter{svc: s, baseURL: baseURL})
	}
	return out
}

func (s *Service) orderedDiscoveryProviders(providers []string) []string {
	if len(providers) < 2 {
		return providers
	}
	type providerSlot struct {
		baseURL string
		index   int
		state   store.DiscoveryProviderState
		ok      bool
	}
	at := s.now().UTC()
	slots := make([]providerSlot, 0, len(providers))
	for index, provider := range providers {
		state, err := s.discoveryProviders.Load(discoverycore.ProviderName(provider))
		slots = append(slots, providerSlot{
			baseURL: provider,
			index:   index,
			state:   state,
			ok:      err == nil,
		})
	}
	sort.SliceStable(slots, func(i, j int) bool {
		left, right := slots[i], slots[j]
		leftCooling := left.ok && left.state.CoolingDown(at)
		rightCooling := right.ok && right.state.CoolingDown(at)
		if leftCooling != rightCooling {
			return !leftCooling
		}
		leftScore := 0.0
		rightScore := 0.0
		if left.ok {
			leftScore = left.state.HealthScore(at)
		}
		if right.ok {
			rightScore = right.state.HealthScore(at)
		}
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return left.index < right.index
	})
	out := make([]string, 0, len(slots))
	for _, slot := range slots {
		out = append(out, slot.baseURL)
	}
	return out
}

func (s *Service) observeDiscoveryProvider(name, outcome string) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(outcome) == "" {
		return
	}
	_, _, _ = s.discoveryProviders.Observe(store.DiscoveryProviderObservation{
		Name:                name,
		Outcome:             outcome,
		FailureCooldown:     time.Duration(s.cfg.Discovery.ProviderFailureCooldownMS) * time.Millisecond,
		BlockedCooldown:     time.Duration(s.cfg.Discovery.ProviderBlockedCooldownMS) * time.Millisecond,
		TimeoutCooldown:     time.Duration(s.cfg.Discovery.ProviderTimeoutCooldownMS) * time.Millisecond,
		UnavailableCooldown: time.Duration(s.cfg.Discovery.ProviderUnavailableCooldownMS) * time.Millisecond,
	})
}
