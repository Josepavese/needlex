package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/document"
	"github.com/josepavese/needlex/internal/pipeline"
	"github.com/josepavese/needlex/internal/proof"
)

func (s *Service) reduce(recorder *proof.Recorder, rawPage pipeline.RawPage) (pipeline.SimplifiedDOM, error) {
	const stage = "reduce"
	if err := recorder.StageStarted(stage, rawPage, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}

	dom, err := s.reducer.Reduce(rawPage)
	if err != nil {
		recorder.Error(stage, "NX_REDUCE_FAILED", err.Error(), nil, s.now().UTC())
		return pipeline.SimplifiedDOM{}, err
	}

	reducedChars := 0
	for _, node := range dom.Nodes {
		reducedChars += len(node.Text)
	}
	metadata := map[string]string{
		"web_ir_version":  core.WebIRVersion,
		"substrate_class": dom.SubstrateClass,
		"reduced_nodes":   fmt.Sprintf("%d", len(dom.Nodes)),
		"reduced_chars":   fmt.Sprintf("%d", reducedChars),
	}
	if strings.TrimSpace(dom.Title) != "" {
		metadata["title"] = dom.Title
	}
	if err := recorder.StageCompleted(stage, dom, len(dom.Nodes), metadata, s.now().UTC()); err != nil {
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
	if req.ObserveAcquisition && strings.TrimSpace(req.Objective) == "" {
		return fmt.Errorf("acquisition observation requires an explicit read objective")
	}
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
	if len(page.Body) > 0 {
		rawIdentity = string(page.Body)
	}
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

func (s *Service) reduceDocument(ctx context.Context, recorder *proof.Recorder, rawPage pipeline.RawPage) (pipeline.SimplifiedDOM, error) {
	const stage = "reduce"
	if err := recorder.StageStarted(stage, rawPage, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}
	limits := pipeline.DocumentLimits{
		MaxBytes:       s.cfg.Runtime.MaxBytes,
		MaxPages:       s.cfg.Document.MaxPages,
		MaxOutputChars: s.cfg.Document.MaxOutputChars,
		TimeoutMS:      s.cfg.Document.TimeoutMS,
	}
	dom, err := document.Extract(ctx, rawPage, limits)
	if err != nil {
		recorder.Error(stage, "NX_DOCUMENT_EXTRACT_FAILED", err.Error(), map[string]string{"format": "pdf"}, s.now().UTC())
		return pipeline.SimplifiedDOM{}, err
	}
	reducedChars := 0
	for _, node := range dom.Nodes {
		reducedChars += len(node.Text)
	}
	metadata := map[string]string{
		"web_ir_version":     core.WebIRVersion,
		"substrate_class":    dom.SubstrateClass,
		"reduced_nodes":      fmt.Sprintf("%d", len(dom.Nodes)),
		"reduced_chars":      fmt.Sprintf("%d", reducedChars),
		"document_format":    "pdf",
		"document_pages":     fmt.Sprintf("%d", dom.Document.PagesTotal),
		"document_processed": fmt.Sprintf("%d", dom.Document.PagesProcessed),
	}
	if strings.TrimSpace(dom.Title) != "" {
		metadata["title"] = dom.Title
	}
	if err := recorder.StageCompleted(stage, dom, len(dom.Nodes), metadata, s.now().UTC()); err != nil {
		return pipeline.SimplifiedDOM{}, err
	}
	return dom, nil
}

func buildDocumentExtraction(dom pipeline.SimplifiedDOM) *core.DocumentExtraction {
	info := dom.Document
	if info == nil {
		return nil
	}
	failures := make([]core.DocumentPageFailure, 0, len(info.PagesFailed))
	for _, failure := range info.PagesFailed {
		failures = append(failures, core.DocumentPageFailure{
			Page:    failure.Page,
			Code:    failure.Code,
			Message: failure.Message,
		})
	}
	return &core.DocumentExtraction{
		Schema:         core.DocumentReadSchemaV1,
		Format:         info.Format,
		RawSHA256:      info.RawSHA256,
		SourceBytes:    info.SourceBytes,
		PagesTotal:     info.PagesTotal,
		PagesProcessed: info.PagesProcessed,
		PagesWithText:  info.PagesWithText,
		PagesFailed:    failures,
		Truncated:      info.Truncated,
		Coverage:       info.Coverage,
		Limits: core.DocumentLimits{
			MaxBytes:       info.Limits.MaxBytes,
			MaxPages:       info.Limits.MaxPages,
			MaxOutputChars: info.Limits.MaxOutputChars,
			TimeoutMS:      info.Limits.TimeoutMS,
		},
	}
}
