package service

import (
	"strings"

	"github.com/josepavese/needlex/internal/core"
	"github.com/josepavese/needlex/internal/pipeline"
)

func buildWebIR(dom pipeline.SimplifiedDOM) core.WebIR {
	nodes := make([]core.WebIRNode, 0, len(dom.Nodes))
	shortText := 0
	heading := 0
	embedded := 0

	for _, node := range dom.Nodes {
		text := strings.TrimSpace(node.Text)
		if text == "" {
			continue
		}
		if len(text) < 48 {
			shortText++
		}
		if node.Kind == "heading" {
			heading++
		}
		if strings.HasPrefix(node.Path, "/embedded/") {
			embedded++
		}
		nodes = append(nodes, core.WebIRNode{
			Path:         node.Path,
			Tag:          node.Tag,
			Kind:         node.Kind,
			Text:         text,
			Depth:        node.Depth,
			HeadingLevel: node.HeadingLevel,
		})
	}

	total := len(nodes)
	shortRatio := 0.0
	headingRatio := 0.0
	if total > 0 {
		shortRatio = float64(shortText) / float64(total)
		headingRatio = float64(heading) / float64(total)
	}

	return core.WebIR{
		Version:   core.WebIRVersion,
		SourceURL: dom.URL,
		Title:     strings.TrimSpace(dom.Title),
		NodeCount: total,
		Nodes:     nodes,
		Signals: core.WebIRSignals{
			ShortTextRatio:    shortRatio,
			HeadingRatio:      headingRatio,
			EmbeddedNodeCount: embedded,
			SubstrateClass:    strings.TrimSpace(dom.SubstrateClass),
		},
	}
}

func ensureMinimumDOM(dom pipeline.SimplifiedDOM) pipeline.SimplifiedDOM {
	if len(dom.Nodes) > 0 || strings.TrimSpace(dom.Title) == "" {
		return dom
	}
	title := strings.TrimSpace(dom.Title)
	dom.Nodes = []pipeline.SimplifiedNode{
		{
			Path:         "/synthetic/title[1]",
			Tag:          "h1",
			Kind:         "heading",
			Text:         title,
			Depth:        1,
			HeadingLevel: 1,
		},
		{
			Path:  "/synthetic/body[1]",
			Tag:   "p",
			Kind:  "paragraph",
			Text:  title,
			Depth: 1,
		},
	}
	return dom
}
