package core

import (
	"fmt"
	"math"
	"strings"
)

const WebIRVersion = "web_ir.v1"

type WebIRNode struct {
	Path         string `json:"path"`
	Tag          string `json:"tag"`
	Kind         string `json:"kind"`
	Text         string `json:"text"`
	Depth        int    `json:"depth"`
	HeadingLevel int    `json:"heading_level,omitempty"`
}

type WebIRSignals struct {
	ShortTextRatio    float64 `json:"short_text_ratio"`
	HeadingRatio      float64 `json:"heading_ratio"`
	EmbeddedNodeCount int     `json:"embedded_node_count"`
	SubstrateClass    string  `json:"substrate_class,omitempty"`
}

type WebIR struct {
	Version   string       `json:"version"`
	SourceURL string       `json:"source_url"`
	Title     string       `json:"title,omitempty"`
	NodeCount int          `json:"node_count"`
	Nodes     []WebIRNode  `json:"nodes"`
	Signals   WebIRSignals `json:"signals"`
}

func (w WebIRNode) Validate() error {
	errs := []error{
		RequireNonEmpty("web_ir.nodes.path", w.Path),
		RequireNonEmpty("web_ir.nodes.tag", w.Tag),
		RequireNonEmpty("web_ir.nodes.kind", w.Kind),
		RequireNonEmpty("web_ir.nodes.text", w.Text),
	}
	if w.Depth <= 0 {
		errs = append(errs, fmt.Errorf("web_ir.nodes.depth must be > 0"))
	}
	if w.HeadingLevel < 0 || w.HeadingLevel > 6 {
		errs = append(errs, fmt.Errorf("web_ir.nodes.heading_level must be between 0 and 6"))
	}
	return JoinErrors(errs...)
}

func (s WebIRSignals) Validate() error {
	errs := []error{}
	if err := validateUnit(s.ShortTextRatio); err != nil {
		errs = append(errs, fmt.Errorf("web_ir.signals.short_text_ratio: %w", err))
	}
	if err := validateUnit(s.HeadingRatio); err != nil {
		errs = append(errs, fmt.Errorf("web_ir.signals.heading_ratio: %w", err))
	}
	if s.EmbeddedNodeCount < 0 {
		errs = append(errs, fmt.Errorf("web_ir.signals.embedded_node_count must be >= 0"))
	}
	if s.SubstrateClass != "" {
		switch s.SubstrateClass {
		case "embedded_app_payload", "theme_heavy_wordpress", "generic_content", "plain_text":
		default:
			errs = append(errs, fmt.Errorf("web_ir.signals.substrate_class %q is not supported", s.SubstrateClass))
		}
	}
	return JoinErrors(errs...)
}

func (w WebIR) Validate() error {
	errs := []error{
		RequireNonEmpty("web_ir.version", w.Version),
		RequireNonEmpty("web_ir.source_url", w.SourceURL),
		w.Signals.Validate(),
	}
	if strings.TrimSpace(w.Version) != WebIRVersion {
		errs = append(errs, fmt.Errorf("web_ir.version must be %q", WebIRVersion))
	}
	if w.NodeCount <= 0 {
		errs = append(errs, fmt.Errorf("web_ir.node_count must be > 0"))
	}
	if len(w.Nodes) == 0 {
		errs = append(errs, fmt.Errorf("web_ir.nodes must not be empty"))
	}
	if w.NodeCount != len(w.Nodes) {
		errs = append(errs, fmt.Errorf("web_ir.node_count must match nodes length"))
	}
	for i, node := range w.Nodes {
		if err := node.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("web_ir.nodes[%d]: %w", i, err))
		}
	}
	return JoinErrors(errs...)
}

func validateUnit(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("must be finite")
	}
	if value < 0 || value > 1 {
		return fmt.Errorf("must be between 0 and 1")
	}
	return nil
}
