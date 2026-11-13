package transport

import (
	"fmt"
	"strings"
	"unicode"

	coreservice "github.com/josepavese/needlex/internal/core/service"
)

// deriveEvidenceCoverage reports limited extraction coverage for the chunks the
// packet actually delivers. It is intentionally conservative: it only fires when
// the delivered body text is negligible (title_only/none) or below a sentence or
// two (thin), so ordinary pages stay free of the field.
func deriveEvidenceCoverage(title string, chunks []coreservice.AgentChunk) *compactEvidence {
	cleanTitle := cleanDisplayString(title)
	deliveredRunes := 0
	bodyRunes := 0
	for _, chunk := range chunks {
		text := cleanDisplayString(chunk.Text)
		if text == "" {
			continue
		}
		deliveredRunes += evidenceAlnumRuneCount(text)
		bodyRunes += evidenceAlnumRuneCount(evidenceBodyResidue(text, cleanTitle, chunk.HeadingPath))
	}
	switch {
	case deliveredRunes == 0:
		return &compactEvidence{
			Coverage: evidenceCoverageNone,
			Reason:   "no usable body text was extracted from the page",
		}
	case bodyRunes < evidenceTitleOnlyBodyRunes && deliveredRunes-bodyRunes > 0 && deliveredRunes-bodyRunes >= bodyRunes:
		return &compactEvidence{
			Coverage: evidenceCoverageTitleOnly,
			Reason:   "delivered evidence is title or heading text only; the page body needed to answer the objective was not extracted",
		}
	case bodyRunes < evidenceThinBodyRunes:
		return &compactEvidence{
			Coverage: evidenceCoverageThin,
			Reason:   fmt.Sprintf("delivered body evidence is thin (%d characters); verify with a targeted read or another source", bodyRunes),
		}
	default:
		return nil
	}
}

// evidenceBodyResidue removes the page title and heading labels from a chunk so
// the remaining alphanumeric characters measure body evidence rather than
// heading structure.
func evidenceBodyResidue(text, title string, headingPath []string) string {
	residue := strings.ToLower(cleanDisplayString(text))
	labels := make([]string, 0, len(headingPath)+1)
	labels = append(labels, title)
	labels = append(labels, headingPath...)
	for _, label := range labels {
		label = strings.ToLower(cleanDisplayString(label))
		if label == "" {
			continue
		}
		residue = strings.ReplaceAll(residue, label, " ")
	}
	return cleanDisplayString(residue)
}

func evidenceAlnumRuneCount(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			count++
		}
	}
	return count
}

func evidenceUncertaintyReason(evidence *compactEvidence) string {
	if evidence == nil {
		return ""
	}
	switch evidence.Coverage {
	case evidenceCoverageTitleOnly:
		return "title_only_extraction"
	case evidenceCoverageThin:
		return "thin_extraction"
	case evidenceCoverageNone:
		return "no_extraction"
	default:
		return ""
	}
}

// evidenceUncertaintyFloor makes limited coverage raise the uncertainty level so
// the packet cannot present title-only extraction with low uncertainty.
func evidenceUncertaintyFloor(level string, evidence *compactEvidence) string {
	if evidence == nil {
		return level
	}
	switch evidence.Coverage {
	case evidenceCoverageTitleOnly, evidenceCoverageNone:
		return "high"
	case evidenceCoverageThin:
		if level == "low" {
			return "medium"
		}
	}
	return level
}
