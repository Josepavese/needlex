package semanticrank

import (
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
)

func semanticRoleText(role string) string {
	switch strings.TrimSpace(role) {
	case "custodian_origin":
		return "Primary source family surface maintained by the responsible entity, institution, publisher, product, project, or authority."
	case "custodian_record":
		return "Maintained authoritative record, documentation, reference, specification, policy, API contract, or canonical knowledge from the responsible family."
	case "derivative_representation":
		return "Derivative or secondary representation, mirror, aggregator, copy, commentary, index, directory, comparison, translation, or external summary."
	case "distribution_node":
		return "Distribution, package, artifact, release, implementation, download, registry, or dependency surface connected to an entity."
	case "social_context":
		return "Social, temporal, discussion, news, blog, support, forum, announcement, issue, or community context around an entity."
	default:
		return "Unclassified retrieval resource with unknown semantic role."
	}
}

func resourceClassText(class string) string {
	switch strings.TrimSpace(class) {
	case discoverycore.ResourceClassHTMLLike:
		return "Human-readable web document or page suitable for source evidence."
	case discoverycore.ResourceClassDocumentFile:
		return "Portable document or publication file that may contain source evidence."
	case discoverycore.ResourceClassStructured:
		return "Structured data resource such as JSON, XML, feed, metadata, or machine-readable record."
	case discoverycore.ResourceClassTextAsset:
		return "Plain text, stylesheet, script, or text asset resource that can be a valid retrieval target when requested."
	case discoverycore.ResourceClassMediaAsset:
		return "Image, audio, video, icon, or media asset resource."
	case discoverycore.ResourceClassArchiveFile:
		return "Archive, package, compressed artifact, binary distribution, or downloadable bundle."
	default:
		return "Unknown resource class."
	}
}
