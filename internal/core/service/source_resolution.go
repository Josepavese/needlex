package service

import (
	"github.com/josepavese/needlex/internal/core/sourceresolution"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

func (s *Service) sourceResolver() sourceresolution.Resolver {
	return sourceresolution.Resolver{
		Config:    s.cfg,
		Acquirer:  s.acquirer,
		Reducer:   s.reducer,
		Segmenter: s.segmenter,
		Renderer:  s.renderer,
		Semantic:  s.semantic,
		Now:       s.now,
		Reduce: func(recorder *proof.Recorder, page pipeline.RawPage) (pipeline.SimplifiedDOM, error) {
			return s.reduce(recorder, page)
		},
	}
}

func sourceResolutionRequest(req ReadRequest) sourceresolution.Request {
	return sourceresolution.Request{
		Objective:         req.Objective,
		UserAgent:         req.UserAgent,
		FetchProfile:      req.FetchProfile,
		FetchRetryProfile: req.FetchRetryProfile,
		RenderHint:        req.RenderHint,
		RenderMode:        req.RenderMode,
		AgentReadableMode: req.AgentReadableMode,
	}
}
