package service

import (
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

func (s *Service) reduce(recorder *proof.Recorder, rawPage pipeline.RawPage, req ReadRequest) (pipeline.SimplifiedDOM, error) {
	const stage = "reduce"
	if err := recorder.StageStarted(stage, rawPage, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}

	dom, err := s.reducer.ReduceProfile(rawPage, req.PruningProfile)
	if err != nil {
		recorder.Error(stage, "NX_REDUCE_FAILED", err.Error(), nil, s.now().UTC())
		return pipeline.SimplifiedDOM{}, err
	}

	reducedChars := 0
	for _, node := range dom.Nodes {
		reducedChars += len(node.Text)
	}
	if err := recorder.StageCompleted(stage, dom, len(dom.Nodes), map[string]string{
		"title":           dom.Title,
		"web_ir_version":  core.WebIRVersion,
		"substrate_class": dom.SubstrateClass,
		"reduced_nodes":   fmt.Sprintf("%d", len(dom.Nodes)),
		"reduced_chars":   fmt.Sprintf("%d", reducedChars),
	}, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}
	return dom, nil
}

func (s *Service) segment(recorder *proof.Recorder, dom pipeline.SimplifiedDOM) ([]pipeline.Segment, error) {
	const stage = "segment"
	if err := recorder.StageStarted(stage, dom, s.now().UTC()); err != nil {
		return nil, err
	}

	segments := s.segmenter.Segment(dom)
	if len(segments) == 0 {
		err := fmt.Errorf("no segments produced")
		recorder.Error(stage, "NX_EMPTY_SEGMENTS", err.Error(), nil, s.now().UTC())
		return nil, err
	}

	if err := recorder.StageCompleted(stage, segments, len(segments), nil, s.now().UTC()); err != nil {
		return nil, err
	}
	return segments, nil
}

func validateReadRequest(req ReadRequest) error {
	if strings.TrimSpace(req.URL) == "" {
		return fmt.Errorf("read request url must not be empty")
	}
	switch strings.ToLower(strings.TrimSpace(req.RenderMode)) {
	case "", "auto", "off", "required":
	default:
		return fmt.Errorf("read request render mode must be one of auto, off, required")
	}
	switch strings.ToLower(strings.TrimSpace(req.AgentReadableMode)) {
	case "", "auto", "declared", "off":
	default:
		return fmt.Errorf("read request agent readable mode must be one of auto, declared, off")
	}
	return nil
}

func buildDocument(page pipeline.RawPage, title string) core.Document {
	rawIdentity := page.HTML
	if strings.TrimSpace(page.NetworkText) != "" {
		rawIdentity += "\n\n" + page.NetworkText
	}
	return core.Document{
		ID:        prefixedHash("doc", page.FinalURL, rawIdentity),
		URL:       page.URL,
		FinalURL:  page.FinalURL,
		Title:     strings.TrimSpace(title),
		FetchedAt: page.FetchedAt,
		FetchMode: page.FetchMode,
		RawHash:   prefixedHash("sha256", rawIdentity),
	}
}
