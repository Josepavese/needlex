package webdiscover

import (
	"errors"
	"strings"

	"github.com/josepavese/needlex/internal/core/failure"
	"github.com/josepavese/needlex/internal/store"
)

type ProviderUnavailableError struct {
	Reason string
}

func (e ProviderUnavailableError) Error() string {
	if strings.TrimSpace(e.Reason) == "" {
		return "provider unavailable"
	}
	return e.Reason
}

func IsProviderUnavailable(err error) bool {
	var unavailable ProviderUnavailableError
	return errors.As(err, &unavailable)
}

func ProviderOutcome(err error) string {
	if err == nil {
		return store.DiscoveryProviderOutcomeSuccess
	}
	class := failure.Classify(err)
	if IsProviderUnavailable(err) || class == failure.ClassUnavailableUpstream {
		return store.DiscoveryProviderOutcomeUnavailable
	}
	switch class {
	case failure.ClassProviderBlocked:
		return store.DiscoveryProviderOutcomeBlocked
	case failure.ClassUpstreamTimeout:
		return store.DiscoveryProviderOutcomeTimeout
	default:
		return store.DiscoveryProviderOutcomeFailure
	}
}

func ProviderLevelFailure(err error) bool {
	return failure.IsProviderLevel(failure.Classify(err)) || IsProviderUnavailable(err)
}
