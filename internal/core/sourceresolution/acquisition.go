package sourceresolution

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/core/webirbuilder"
	"golang.org/x/net/html"

	"github.com/josepavese/needlex/internal/core/agentreadable"
	"github.com/josepavese/needlex/internal/intel"
	"github.com/josepavese/needlex/internal/pipeline"
)

// AcquisitionInput reconstructs the original HTTP decision point after the read
// completes. It performs no I/O or model work and never consumes live read time.
// remaining is the budget observed at that original point; nil means no
// deadline. Later operational outcomes must not become model features.
func (r Resolver) AcquisitionInput(req Request, page pipeline.RawPage, remaining *time.Duration) (intel.AcquisitionInput, error) {
	input := intel.AcquisitionInput{SourcePartial: page.Partial, Objective: req.Objective, URL: page.FinalURL, ContentType: page.ContentType, SourceHash: fmt.Sprintf("%x", sha256.Sum256([]byte(page.HTML)))}
	if input.URL == "" {
		input.URL = page.URL
	}
	mode := normalizeRenderMode(req.RenderMode, req.RenderHint)
	if mode == "required" {
		input.AllowedActions = []string{intel.AcquisitionRender}
		return input, nil
	}
	input.AllowedActions = []string{intel.AcquisitionServe, intel.AcquisitionInsufficient}
	if pipeline.IsHTMLLikeRawPage(page) {
		input.HasScriptElements = hasScriptElement(page.HTML)
		if r.Config.Agent.ReadableEnabled && normalizeAgentReadableMode(req.AgentReadableMode) != "off" {
			for _, source := range agentReadableCandidateBatch(declaredAgentReadableCandidates(agentreadable.Discover(page, 0)), agentReadableCandidateLimit(r.Config.Agent.MaxCandidates)) {
				input.DeclaredSources = append(input.DeclaredSources, intel.AcquisitionSource{URL: source.URL, Kind: source.Kind, DeclaredBy: source.DeclaredBy})
			}
			if len(input.DeclaredSources) > 0 && (remaining == nil || *remaining >= agentReadableProbeMinRemaining) {
				input.AllowedActions = append(input.AllowedActions, intel.AcquisitionDeclared)
			}
		}
		if mode != "off" && r.Config.Render.Enabled && (remaining == nil || *remaining >= AutoRenderDeadlineMinRemaining) {
			input.AllowedActions = append(input.AllowedActions, intel.AcquisitionRender)
		}
	}
	dom, err := r.Reducer.Reduce(page)
	if err != nil {
		return input, err
	}
	dom = webirbuilder.EnsureMinimum(dom)
	input.SourceClass = dom.SubstrateClass
	for _, segment := range r.Segmenter.Segment(dom) {
		input.Evidence = append(input.Evidence, intel.DecisionEvidence{ID: input.SourceHash + "#" + strings.Join(segment.NodePaths, "|"), Heading: segment.HeadingPath, Text: segment.Text})
	}
	return input, nil
}

func hasScriptElement(source string) bool {
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return false
	}
	var walk func(*html.Node) bool
	walk = func(node *html.Node) bool {
		if node.Type == html.ElementNode && node.Data == "script" {
			return true
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if walk(child) {
				return true
			}
		}
		return false
	}
	return walk(doc)
}
