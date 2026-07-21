package fetchpolicy

import (
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/pipeline"
)

func Input(cfg config.Config, rawURL, userAgent, fetchProfile, fetchRetryProfile, accept string) pipeline.AcquireInput {
	if fetchProfile == "" {
		fetchProfile = cfg.Fetch.Profile
	}
	if fetchRetryProfile == "" {
		fetchRetryProfile = cfg.Fetch.RetryProfile
	}
	return pipeline.AcquireInput{
		URL:                 rawURL,
		Timeout:             time.Duration(cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:            cfg.Runtime.MaxBytes,
		UserAgent:           userAgent,
		Accept:              accept,
		Profile:             fetchProfile,
		RetryProfile:        fetchRetryProfile,
		BlockedRetryBackoff: time.Duration(cfg.Fetch.BlockedRetryBackoffMS) * time.Millisecond,
		BlockedRetryJitter:  time.Duration(cfg.Fetch.BlockedRetryJitterMS) * time.Millisecond,
		PerHostMinGap:       time.Duration(cfg.Fetch.PerHostMinGapMS) * time.Millisecond,
		PerHostJitter:       time.Duration(cfg.Fetch.PerHostJitterMS) * time.Millisecond,
		TimeoutRetryBackoff: time.Duration(cfg.Fetch.TimeoutRetryBackoffMS) * time.Millisecond,
		TimeoutRetryJitter:  time.Duration(cfg.Fetch.TimeoutRetryJitterMS) * time.Millisecond,
		AllowPartial:        true,
	}
}
