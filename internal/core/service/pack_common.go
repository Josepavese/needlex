package service

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
)

func resolveProfile(profile string) (string, error) {
	profile = strings.TrimSpace(strings.ToLower(profile))
	if profile == "" {
		return core.ProfileStandard, nil
	}
	switch profile {
	case core.ProfileTiny, core.ProfileStandard, core.ProfileDeep:
		return profile, nil
	default:
		return "", fmt.Errorf("unsupported profile %q", profile)
	}
}

func objectiveOrDefault(objective string) string {
	if strings.TrimSpace(objective) == "" {
		return "read"
	}
	return objective
}

func deltaClassFromHits(stableHits, novelHits int) string {
	switch {
	case stableHits > 0 && novelHits == 0:
		return "stable"
	case stableHits == 0 && novelHits > 0:
		return "changed"
	default:
		return "mixed"
	}
}

func reuseMode(stable []string) string {
	if len(stable) == 0 {
		return "fresh"
	}
	return "delta_aware"
}
