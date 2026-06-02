package service

import (
	"time"

	"github.com/josepavese/needlex/internal/pipeline"
)

func (s *Service) fetchAcquireInput(rawURL, userAgent string) pipeline.AcquireInput {
	return s.fetchAcquireInputWithProfiles(rawURL, userAgent, "", "")
}

func (s *Service) fetchAcquireInputWithProfiles(rawURL, userAgent, fetchProfile, fetchRetryProfile string) pipeline.AcquireInput {
	return s.fetchAcquireInputWithProfilesAndAccept(rawURL, userAgent, fetchProfile, fetchRetryProfile, "")
}

func (s *Service) fetchAcquireInputWithProfilesAndAccept(rawURL, userAgent, fetchProfile, fetchRetryProfile, accept string) pipeline.AcquireInput {
	if fetchProfile == "" {
		fetchProfile = s.cfg.Fetch.Profile
	}
	if fetchRetryProfile == "" {
		fetchRetryProfile = s.cfg.Fetch.RetryProfile
	}
	return pipeline.AcquireInput{
		URL:                 rawURL,
		Timeout:             time.Duration(s.cfg.Runtime.TimeoutMS) * time.Millisecond,
		MaxBytes:            s.cfg.Runtime.MaxBytes,
		UserAgent:           userAgent,
		Accept:              accept,
		Profile:             fetchProfile,
		RetryProfile:        fetchRetryProfile,
		BlockedRetryBackoff: time.Duration(s.cfg.Fetch.BlockedRetryBackoffMS) * time.Millisecond,
		BlockedRetryJitter:  time.Duration(s.cfg.Fetch.BlockedRetryJitterMS) * time.Millisecond,
		PerHostMinGap:       time.Duration(s.cfg.Fetch.PerHostMinGapMS) * time.Millisecond,
		PerHostJitter:       time.Duration(s.cfg.Fetch.PerHostJitterMS) * time.Millisecond,
		TimeoutRetryBackoff: time.Duration(s.cfg.Fetch.TimeoutRetryBackoffMS) * time.Millisecond,
		TimeoutRetryJitter:  time.Duration(s.cfg.Fetch.TimeoutRetryJitterMS) * time.Millisecond,
		AllowPartial:        true,
	}
}
