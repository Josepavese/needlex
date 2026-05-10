package candidateintel

import (
	"context"
	"fmt"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

type candidateCluster struct {
	ID         string
	MemberIdx  []int
	Family     string
	Coherence  float64
	Centrality map[int]float64
}

type candidateIntelligenceScorecard struct {
	Similarity     float64
	HostSimilarity float64
	PageSimilarity float64
	Centrality     float64
	Role           candidateRoleEvidence
	Boost          float64
	Reasons        []string
}

type candidateRoleEvidence struct {
	Role                string
	Confidence          float64
	Intent              float64
	OriginAlignment     float64
	DerivativeAlignment float64
	Boost               float64
	Reasons             []string
}

type semanticRoleProfile struct {
	ID   string
	Text string
}

const (
	roleCustodianOrigin  = "custodian_origin"
	roleCustodianRecord  = "custodian_record"
	roleDerivative       = "derivative_representation"
	roleDistributionNode = "distribution_node"
	roleSocialContext    = "social_context"
)

func Apply(ctx context.Context, semantic intel.SemanticAligner, goal string, candidates []discoverycore.Candidate) []discoverycore.Candidate {
	window := Window(candidates)
	if window < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	annotated := append([]discoverycore.Candidate{}, candidates...)
	texts := make([]intel.SemanticCandidate, 0, window)
	identityTexts := make([]intel.SemanticCandidate, 0, window)
	hostTexts := make([]intel.SemanticCandidate, 0, window)
	pageTexts := make([]intel.SemanticCandidate, 0, window)
	for i := 0; i < window; i++ {
		annotated[i].Metadata = discoverycore.MergeMetadata(annotated[i].Metadata, map[string]string{
			"resource_class": discoverycore.FirstNonEmpty(annotated[i].Metadata["resource_class"], discoverycore.ResourceClass(annotated[i].URL)),
		})
		texts = append(texts, intel.SemanticCandidate{
			ID:   annotated[i].URL,
			Text: candidateSemanticText(annotated[i]),
		})
		identityTexts = append(identityTexts, intel.SemanticCandidate{
			ID: annotated[i].URL,
			Text: discoverycore.JoinNonEmpty(
				compactSemanticText(annotated[i].Metadata["host_root_title"], 160),
				compactSemanticText(annotated[i].Metadata["host_root_context"], 260),
				compactSemanticText(annotated[i].Metadata["page_title"], 160),
				compactSemanticText(annotated[i].Label, 160),
			),
		})
		hostTexts = append(hostTexts, intel.SemanticCandidate{
			ID:   annotated[i].URL,
			Text: discoverycore.JoinNonEmpty(compactSemanticText(annotated[i].Metadata["host_root_title"], 160)),
		})
		pageTexts = append(pageTexts, intel.SemanticCandidate{
			ID: annotated[i].URL,
			Text: discoverycore.JoinNonEmpty(
				compactSemanticText(annotated[i].Metadata["page_title"], 160),
				compactSemanticText(annotated[i].Label, 160),
			),
		})
	}

	goalSimilarity := scoreCandidateSetToGoal(ctx, semantic, goal, texts)
	for url, value := range scoreCandidateSetToGoal(ctx, semantic, goal, identityTexts) {
		if value > goalSimilarity[url] {
			goalSimilarity[url] = value
		}
	}
	hostSimilarity := scoreCandidateSetToGoal(ctx, semantic, goal, hostTexts)
	pageSimilarity := scoreCandidateSetToGoal(ctx, semantic, goal, pageTexts)
	if len(goalSimilarity) == 0 {
		return candidates
	}
	roleEvidence := classifyCandidateSemanticRoles(ctx, semantic, goal, texts)
	graph := buildCandidateSemanticGraph(ctx, semantic, texts, annotated[:window])
	clusters := buildCandidateClusters(annotated[:window], goalSimilarity, graph)
	clusterByIdx := map[int]candidateCluster{}
	for _, cluster := range clusters {
		for _, idx := range cluster.MemberIdx {
			clusterByIdx[idx] = cluster
		}
	}

	for i := 0; i < window; i++ {
		cluster := clusterByIdx[i]
		card := scoreCandidateIntelligence(annotated[i], cluster, i, goalSimilarity, hostSimilarity, pageSimilarity, roleEvidence)
		applyCandidateIntelligenceScore(&annotated[i], card)
		annotated[i].Metadata = discoverycore.MergeMetadata(annotated[i].Metadata, candidateIntelligenceMetadata(cluster, card))
	}
	applyClusterRepresentativeSelection(annotated[:window], clusterByIdx, goalSimilarity)
	discoverycore.SortCandidates(annotated)
	return annotated
}

func scoreCandidateIntelligence(
	candidate discoverycore.Candidate,
	cluster candidateCluster,
	idx int,
	goalSimilarity map[string]float64,
	hostSimilarity map[string]float64,
	pageSimilarity map[string]float64,
	roleEvidence map[string]candidateRoleEvidence,
) candidateIntelligenceScorecard {
	card := candidateIntelligenceScorecard{
		Similarity:     goalSimilarity[candidate.URL],
		HostSimilarity: hostSimilarity[candidate.URL],
		PageSimilarity: pageSimilarity[candidate.URL],
		Centrality:     cluster.Centrality[idx],
		Role:           roleEvidence[candidate.URL],
	}
	card.Boost = min(card.Similarity*0.20, 0.14)
	card.Boost += clusterIntelligenceBoost(cluster, card.Centrality)
	card.Boost += resourceClassIntelligenceBoost(candidate.Metadata["resource_class"])
	card.Boost += card.Role.Boost
	card.Reasons = append(card.Reasons, card.Role.Reasons...)
	if boost, reason := identityAlignmentBoost(card.PageSimilarity, card.HostSimilarity); reason != "" {
		card.Boost += boost
		card.Reasons = append(card.Reasons, reason)
	}
	card.Reasons = append(card.Reasons, intelligenceBoostReasons(cluster, card)...)
	return card
}

func clusterIntelligenceBoost(cluster candidateCluster, centrality float64) float64 {
	boost := 0.0
	if cluster.ClusterSize() > 1 {
		boost += 0.04 * float64(min(cluster.ClusterSize()-1, 2))
	}
	if cluster.Coherence > 0 {
		boost += min(cluster.Coherence*0.07, 0.08)
	}
	if centrality > 0 {
		boost += min(centrality*0.14, 0.16)
	}
	return boost
}

func resourceClassIntelligenceBoost(class string) float64 {
	switch class {
	case discoverycore.ResourceClassHTMLLike:
		return 0.03
	case discoverycore.ResourceClassMediaAsset:
		return -0.12
	case discoverycore.ResourceClassArchiveFile:
		return -0.08
	case discoverycore.ResourceClassStructured:
		return -0.02
	case discoverycore.ResourceClassTextAsset:
		return -0.01
	default:
		return 0
	}
}

func identityAlignmentBoost(pageSim, hostSim float64) (float64, string) {
	if pageSim >= 0.24 && hostSim >= 0.22 {
		return 0.12, "candidate_identity_alignment"
	}
	if pageSim >= 0.24 && hostSim > 0 && hostSim < 0.20 {
		return -0.18, "candidate_identity_mismatch"
	}
	return 0, ""
}

func intelligenceBoostReasons(cluster candidateCluster, card candidateIntelligenceScorecard) []string {
	if card.Boost == 0 {
		return nil
	}
	reasons := []string{"candidate_intelligence"}
	if cluster.ClusterSize() > 1 {
		reasons = append(reasons, "candidate_cluster_support")
	}
	if card.Similarity > 0 {
		reasons = append(reasons, "candidate_goal_grounding")
	}
	if card.Centrality > 0 {
		reasons = append(reasons, "candidate_graph_centrality")
	}
	return reasons
}

func applyCandidateIntelligenceScore(candidate *discoverycore.Candidate, card candidateIntelligenceScorecard) {
	if card.Boost != 0 {
		candidate.Score += card.Boost
	}
	for _, reason := range card.Reasons {
		candidate.Reason = discoverycore.AppendUniqueReason(candidate.Reason, reason)
	}
}

func candidateIntelligenceMetadata(cluster candidateCluster, card candidateIntelligenceScorecard) map[string]string {
	return map[string]string{
		"candidate_goal_similarity":     fmt.Sprintf("%.3f", card.Similarity),
		"candidate_host_similarity":     fmt.Sprintf("%.3f", card.HostSimilarity),
		"candidate_page_similarity":     fmt.Sprintf("%.3f", card.PageSimilarity),
		"cluster_id":                    cluster.ID,
		"cluster_size":                  fmt.Sprintf("%d", cluster.ClusterSize()),
		"cluster_family":                cluster.Family,
		"cluster_coherence":             fmt.Sprintf("%.3f", cluster.Coherence),
		"cluster_centrality":            fmt.Sprintf("%.3f", card.Centrality),
		"semantic_role":                 card.Role.Role,
		"semantic_role_confidence":      fmt.Sprintf("%.3f", card.Role.Confidence),
		"semantic_role_intent":          fmt.Sprintf("%.3f", card.Role.Intent),
		"semantic_origin_alignment":     fmt.Sprintf("%.3f", card.Role.OriginAlignment),
		"semantic_derivative_alignment": fmt.Sprintf("%.3f", card.Role.DerivativeAlignment),
	}
}

func Window(candidates []discoverycore.Candidate) int {
	if len(candidates) < 2 {
		return 0
	}
	if shouldReviewCandidateWindow(candidates) {
		return min(len(candidates), 8)
	}
	return 0
}

func shouldReviewCandidateWindow(candidates []discoverycore.Candidate) bool {
	if candidates[0].Score-candidates[1].Score <= 0.42 {
		return true
	}
	limit := min(len(candidates), 8)
	topStrong := hasSemanticProvenance(candidates[0])
	for i := 1; i < limit; i++ {
		if hasSemanticProvenance(candidates[i]) && !topStrong && candidates[0].Score-candidates[i].Score <= 1.25 {
			return true
		}
		if hasSemanticProvenance(candidates[i]) && topStrong && candidates[0].Score-candidates[i].Score <= 1.40 {
			return true
		}
	}
	return false
}

func hasSemanticProvenance(candidate discoverycore.Candidate) bool {
	for _, reason := range candidate.Reason {
		switch strings.TrimSpace(reason) {
		case "semantic_root_identity_probe",
			"host_root_identity_probe",
			"host_root_candidate",
			"identity_reference",
			"semantic_family_alignment",
			"semantic_custodian_alignment",
			"semantic_quorum_provider_fusion":
			return true
		}
	}
	return false
}

func candidateSemanticText(candidate discoverycore.Candidate) string {
	return discoverycore.JoinNonEmpty(
		compactSemanticText(candidate.Metadata["host_root_title"], 160),
		compactSemanticText(candidate.Metadata["host_root_context"], 260),
		compactSemanticText(candidate.Metadata["page_title"], 160),
		compactSemanticText(candidate.Metadata["source_context"], 220),
		compactSemanticText(candidate.Metadata["web_ir_context"], 320),
		compactSemanticText(candidate.Label, 160),
		candidate.Metadata["resource_class"],
	)
}

func compactSemanticText(value string, maxRunes int) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if maxRunes <= 0 || len([]rune(clean)) <= maxRunes {
		return clean
	}
	runes := []rune(clean)
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func semanticRoleProfiles() []semanticRoleProfile {
	return []semanticRoleProfile{
		{
			ID: roleCustodianOrigin,
			Text: "Primary custodian origin surface. The entity, institution, standard body, project, service, publisher, or product speaks for itself through its own identity, ownership, provenance, and maintained presence. " +
				"Superficie oficial primaria, source de référence, fonte autoritativa, sumber resmi, página principal, 根源となる公式情報.",
		},
		{
			ID: roleCustodianRecord,
			Text: "Authoritative maintained record from the same custodian family: reference, manual, specification, policy, API contract, technical record, standard, documentation, canonical knowledge maintained by the responsible entity. " +
				"Documentación oficial, documentazione autorevole, technische Referenz, spécification maintenue, 公式ドキュメント.",
		},
		{
			ID: roleDerivative,
			Text: "Derivative representation of another entity: mirror, aggregator, republished copy, commentary, comparison, directory, index, review, summary, translation, curated or secondary explanation about a different source. " +
				"Documentation browser that collects references from many projects, generated knowledge base, raccolta di terze parti, réplica, resumen externo, índice derivado, 非一次情報.",
		},
		{
			ID:   roleDistributionNode,
			Text: "Distribution or implementation node: source archive, package registry, artifact catalog, download listing, release channel, install feed, binary or dependency distribution surface connected to an entity.",
		},
		{
			ID:   roleSocialContext,
			Text: "Social or temporal context surface: forum, discussion, support conversation, news article, blog post, issue thread, community answer, announcement, or commentary around an entity.",
		},
	}
}

func semanticRoleCandidates() []intel.SemanticCandidate {
	profiles := semanticRoleProfiles()
	out := make([]intel.SemanticCandidate, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, intel.SemanticCandidate{ID: profile.ID, Text: compactSemanticText(profile.Text, 520)})
	}
	return out
}

func classifyCandidateSemanticRoles(ctx context.Context, semantic intel.SemanticAligner, goal string, candidates []intel.SemanticCandidate) map[string]candidateRoleEvidence {
	if len(candidates) == 0 || strings.TrimSpace(goal) == "" {
		return nil
	}
	roleCandidates := semanticRoleCandidates()
	roleIntent := scoreCandidateSetToGoal(ctx, semantic, goal, roleCandidates)
	if len(roleIntent) == 0 {
		return nil
	}
	roleScores := map[string]map[string]float64{}
	if cross, ok := semantic.(intel.SemanticCrossScorer); ok {
		if scores, err := cross.ScoreCross(ctx, roleCandidates, candidates); err == nil {
			roleScores = scores
		}
	}
	if len(roleScores) == 0 {
		for _, role := range roleCandidates {
			scores := scoreCandidateSetToGoal(ctx, semantic, role.Text, candidates)
			if len(scores) > 0 {
				roleScores[role.ID] = scores
			}
		}
	}
	if len(roleScores) == 0 {
		return nil
	}
	out := make(map[string]candidateRoleEvidence, len(candidates))
	for _, candidate := range candidates {
		out[candidate.ID] = candidateRoleScore(candidate.ID, roleIntent, roleScores)
	}
	return out
}

func candidateRoleScore(candidateID string, roleIntent map[string]float64, roleScores map[string]map[string]float64) candidateRoleEvidence {
	evidence := candidateRoleEvidence{}
	for roleID, scores := range roleScores {
		score := scores[candidateID]
		if score > evidence.Confidence {
			evidence.Role = roleID
			evidence.Confidence = score
			evidence.Intent = roleIntent[roleID]
		}
	}
	if evidence.Confidence <= 0 {
		return candidateRoleEvidence{}
	}
	originRoleScore := max(roleScores[roleCustodianOrigin][candidateID], roleScores[roleCustodianRecord][candidateID])
	originIntent := max(roleIntent[roleCustodianOrigin], roleIntent[roleCustodianRecord])
	derivativeRoleScore := max(roleScores[roleDerivative][candidateID], roleScores[roleSocialContext][candidateID])
	derivativeIntent := max(roleIntent[roleDerivative], roleIntent[roleSocialContext])
	distributionIntent := roleIntent[roleDistributionNode]
	distributionAlignment := roleScores[roleDistributionNode][candidateID] * distributionIntent

	evidence.OriginAlignment = originRoleScore * originIntent
	evidence.DerivativeAlignment = derivativeRoleScore * derivativeIntent
	evidence.Boost, evidence.Reasons = semanticRoleBoost(evidence, originIntent, derivativeIntent, distributionIntent, distributionAlignment)
	return evidence
}

func semanticRoleBoost(evidence candidateRoleEvidence, originIntent, derivativeIntent, distributionIntent, distributionAlignment float64) (float64, []string) {
	boost := 0.0
	reasons := []string{}
	switch evidence.Role {
	case roleCustodianOrigin, roleCustodianRecord:
		if evidence.OriginAlignment > 0 && evidence.OriginAlignment >= evidence.DerivativeAlignment+0.006 {
			boost += min(evidence.OriginAlignment*1.25+originIntent*0.08, 0.46)
			reasons = append(reasons, "semantic_custodian_alignment")
		}
	case roleDerivative, roleSocialContext:
		if originIntent >= derivativeIntent-0.02 || evidence.DerivativeAlignment >= evidence.OriginAlignment {
			boost -= min((evidence.Confidence*0.45)+(max(0, evidence.DerivativeAlignment-evidence.OriginAlignment)*1.20), 0.38)
			reasons = append(reasons, "semantic_derivative_surface_penalty")
		}
	}
	if evidence.Role != roleCustodianOrigin && evidence.Role != roleCustodianRecord &&
		evidence.DerivativeAlignment > evidence.OriginAlignment+0.025 && originIntent > derivativeIntent+0.015 {
		boost -= min((evidence.DerivativeAlignment-evidence.OriginAlignment)*0.80+originIntent*0.04, 0.24)
		if !containsReason(reasons, "semantic_derivative_surface_penalty") {
			reasons = append(reasons, "semantic_derivative_surface_penalty")
		}
	}
	if distributionAlignment > evidence.OriginAlignment+0.025 && distributionAlignment > evidence.DerivativeAlignment && distributionIntent >= originIntent-0.01 {
		boost += min(distributionAlignment*0.35, 0.12)
		reasons = append(reasons, "semantic_distribution_alignment")
	} else if distributionAlignment > evidence.OriginAlignment+0.02 && originIntent > distributionIntent+0.015 {
		boost -= min((distributionAlignment-evidence.OriginAlignment)*0.80+originIntent*0.05, 0.24)
		reasons = append(reasons, "semantic_distribution_surface_penalty")
	}
	return boost, reasons
}

func containsReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func scoreCandidateSetToGoal(ctx context.Context, semantic intel.SemanticAligner, goal string, candidates []intel.SemanticCandidate) map[string]float64 {
	if len(candidates) == 0 || strings.TrimSpace(goal) == "" {
		return nil
	}
	scored, err := semantic.Score(ctx, goal, candidates)
	if err != nil || len(scored) == 0 {
		return nil
	}
	out := make(map[string]float64, len(scored))
	for _, item := range scored {
		out[item.ID] = max(item.Similarity, 0)
	}
	return out
}

type candidateSemanticGraph struct {
	weights map[int]map[int]float64
}

func buildCandidateSemanticGraph(ctx context.Context, semantic intel.SemanticAligner, texts []intel.SemanticCandidate, candidates []discoverycore.Candidate) candidateSemanticGraph {
	graph := candidateSemanticGraph{weights: map[int]map[int]float64{}}
	if len(texts) < 2 {
		return graph
	}
	raw := candidateSemanticSimilarityMatrix(ctx, semantic, texts)
	for i := 0; i < len(texts); i++ {
		for j := i + 1; j < len(texts); j++ {
			similarity := mutualSemanticSimilarity(raw, i, j)
			structural := candidateStructuralAffinity(candidates[i], candidates[j])
			weight := similarity*0.66 + structural*0.34
			if weight <= 0 {
				continue
			}
			if graph.weights[i] == nil {
				graph.weights[i] = map[int]float64{}
			}
			if graph.weights[j] == nil {
				graph.weights[j] = map[int]float64{}
			}
			graph.weights[i][j] = weight
			graph.weights[j][i] = weight
		}
	}
	return graph
}

func candidateSemanticSimilarityMatrix(ctx context.Context, semantic intel.SemanticAligner, texts []intel.SemanticCandidate) [][]float64 {
	raw := make([][]float64, len(texts))
	indexByID := map[string]int{}
	for idx, candidate := range texts {
		indexByID[candidate.ID] = idx
	}
	if cross, ok := semantic.(intel.SemanticCrossScorer); ok {
		scores, err := cross.ScoreCross(ctx, texts, texts)
		if err == nil && len(scores) > 0 {
			for leftID, rowScores := range scores {
				leftIdx, ok := indexByID[leftID]
				if !ok {
					continue
				}
				row := make([]float64, len(texts))
				for rightID, score := range rowScores {
					if rightIdx, ok := indexByID[rightID]; ok {
						row[rightIdx] = max(score, 0)
					}
				}
				raw[leftIdx] = row
			}
			return raw
		}
	}
	for i := range texts {
		scores, err := semantic.Score(ctx, texts[i].Text, texts)
		if err != nil || len(scores) == 0 {
			continue
		}
		row := make([]float64, len(texts))
		for _, score := range scores {
			if idx, ok := indexByID[score.ID]; ok {
				row[idx] = max(score.Similarity, 0)
			}
		}
		raw[i] = row
	}
	return raw
}

func mutualSemanticSimilarity(raw [][]float64, left, right int) float64 {
	if left >= len(raw) || right >= len(raw) {
		return 0
	}
	var lr, rl float64
	if row := raw[left]; right < len(row) {
		lr = row[right]
	}
	if row := raw[right]; left < len(row) {
		rl = row[left]
	}
	if lr == 0 && rl == 0 {
		return 0
	}
	return (lr + rl) / 2
}

func candidateStructuralAffinity(left, right discoverycore.Candidate) float64 {
	affinity := 0.0
	if sameCandidateFamily(left.URL, right.URL) {
		affinity = max(affinity, 1.0)
	}
	if sameDiscoverHost(left.URL, right.URL) {
		affinity = max(affinity, 0.95)
	}
	if left.Metadata["host_root_title"] != "" && left.Metadata["host_root_title"] == right.Metadata["host_root_title"] {
		affinity = max(affinity, 0.72)
	}
	if left.Metadata["resource_class"] != "" && left.Metadata["resource_class"] == right.Metadata["resource_class"] {
		affinity = max(affinity, 0.28)
	}
	return affinity
}

func sameCandidateFamily(leftURL, rightURL string) bool {
	leftDomain, leftErr := discoverycore.RegistrableDomain(leftURL)
	rightDomain, rightErr := discoverycore.RegistrableDomain(rightURL)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return leftDomain == rightDomain
}

func sameDiscoverHost(leftURL, rightURL string) bool {
	leftHost, leftOK := discoverycore.Hostname(leftURL)
	rightHost, rightOK := discoverycore.Hostname(rightURL)
	return leftOK && rightOK && leftHost == rightHost
}

func candidateFamily(rawURL string) (string, bool) {
	if family, err := discoverycore.RegistrableDomain(rawURL); err == nil && strings.TrimSpace(family) != "" {
		return family, true
	}
	if host, ok := discoverycore.Hostname(rawURL); ok {
		return host, true
	}
	return "", false
}

func buildCandidateClusters(candidates []discoverycore.Candidate, goalSimilarity map[string]float64, graph candidateSemanticGraph) []candidateCluster {
	if len(candidates) == 0 {
		return nil
	}
	uf := newCandidateUnionFind(len(candidates))
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if shouldUnionCandidates(candidates[i], candidates[j], graph.weights[i][j], goalSimilarity) {
				uf.union(i, j)
			}
		}
	}
	groups := uf.groups(len(candidates))
	out := make([]candidateCluster, 0, len(groups))
	seq := 1
	for _, idxs := range groups {
		out = append(out, buildCandidateCluster(seq, idxs, candidates, goalSimilarity, graph))
		seq++
	}
	return out
}

type candidateUnionFind struct {
	parent []int
}

func newCandidateUnionFind(size int) candidateUnionFind {
	parent := make([]int, size)
	for i := range parent {
		parent[i] = i
	}
	return candidateUnionFind{parent: parent}
}

func (u candidateUnionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u candidateUnionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}

func (u candidateUnionFind) groups(size int) map[int][]int {
	groups := map[int][]int{}
	for i := 0; i < size; i++ {
		groups[u.find(i)] = append(groups[u.find(i)], i)
	}
	return groups
}

func shouldUnionCandidates(left, right discoverycore.Candidate, edgeWeight float64, goalSimilarity map[string]float64) bool {
	if sameCandidateFamily(left.URL, right.URL) || edgeWeight >= 0.68 {
		return true
	}
	return goalSimilarity[left.URL] >= 0.28 && goalSimilarity[right.URL] >= 0.28 && edgeWeight >= 0.48
}

func buildCandidateCluster(seq int, idxs []int, candidates []discoverycore.Candidate, goalSimilarity map[string]float64, graph candidateSemanticGraph) candidateCluster {
	return candidateCluster{
		ID:         fmt.Sprintf("cluster_%02d", seq),
		MemberIdx:  idxs,
		Family:     dominantClusterFamily(idxs, candidates),
		Coherence:  clusterCoherence(idxs, candidates, goalSimilarity),
		Centrality: clusterCentrality(idxs, graph),
	}
}

func dominantClusterFamily(idxs []int, candidates []discoverycore.Candidate) string {
	familyCounts := map[string]int{}
	for _, idx := range idxs {
		if family, ok := candidateFamily(candidates[idx].URL); ok {
			familyCounts[family]++
		}
	}
	family := ""
	familyCount := -1
	for candidateFamily, count := range familyCounts {
		if count > familyCount {
			family = candidateFamily
			familyCount = count
		}
	}
	return family
}

func clusterCoherence(idxs []int, candidates []discoverycore.Candidate, goalSimilarity map[string]float64) float64 {
	sum := 0.0
	for _, idx := range idxs {
		sum += goalSimilarity[candidates[idx].URL]
	}
	return sum / float64(len(idxs))
}

func clusterCentrality(idxs []int, graph candidateSemanticGraph) map[int]float64 {
	centrality := map[int]float64{}
	for _, idx := range idxs {
		for _, other := range idxs {
			if idx != other {
				centrality[idx] += graph.weights[idx][other]
			}
		}
	}
	return normalizeCentrality(centrality)
}

func normalizeCentrality(centrality map[int]float64) map[int]float64 {
	maxCentrality := 0.0
	for _, value := range centrality {
		maxCentrality = max(maxCentrality, value)
	}
	if maxCentrality == 0 {
		return centrality
	}
	for idx, value := range centrality {
		centrality[idx] = value / maxCentrality
	}
	return centrality
}

func applyClusterRepresentativeSelection(candidates []discoverycore.Candidate, clusterByIdx map[int]candidateCluster, goalSimilarity map[string]float64) {
	seen := map[string]struct{}{}
	for idx, cluster := range clusterByIdx {
		if _, ok := seen[cluster.ID]; ok {
			continue
		}
		seen[cluster.ID] = struct{}{}
		if cluster.ClusterSize() < 2 {
			continue
		}
		repIdx := idx
		repScore := clusterRepresentativeScore(candidates[idx], cluster, idx, goalSimilarity)
		for _, memberIdx := range cluster.MemberIdx {
			score := clusterRepresentativeScore(candidates[memberIdx], cluster, memberIdx, goalSimilarity)
			if score > repScore {
				repIdx = memberIdx
				repScore = score
			}
		}
		for _, memberIdx := range cluster.MemberIdx {
			if memberIdx == repIdx {
				candidates[memberIdx].Score += 0.12
				candidates[memberIdx].Reason = discoverycore.AppendUniqueReason(candidates[memberIdx].Reason, "candidate_cluster_representative")
				continue
			}
			candidates[memberIdx].Score -= 0.07
			candidates[memberIdx].Reason = discoverycore.AppendUniqueReason(candidates[memberIdx].Reason, "candidate_cluster_redundant")
		}
	}
}

func clusterRepresentativeScore(candidate discoverycore.Candidate, cluster candidateCluster, idx int, goalSimilarity map[string]float64) float64 {
	score := candidate.Score
	if centrality, ok := cluster.Centrality[idx]; ok {
		score += centrality * 0.30
	}
	if similarity := goalSimilarity[candidate.URL]; similarity > 0 {
		score += min(similarity*0.75, 0.45)
	}
	if strings.TrimSpace(candidate.Metadata["embedded_url_source"]) != "" {
		score += 0.35
	}
	depth := discoverycore.URLPathDepth(candidate.URL)
	switch {
	case depth == 0:
		score += 0.08
	case depth == 1:
		score += 0.04
	case depth >= 4:
		score -= 0.04
	}
	switch candidate.Metadata["resource_class"] {
	case discoverycore.ResourceClassHTMLLike:
		score += 0.05
	case discoverycore.ResourceClassMediaAsset:
		score -= 0.18
	case discoverycore.ResourceClassArchiveFile:
		score -= 0.10
	}
	return score
}

func (c candidateCluster) ClusterSize() int { return len(c.MemberIdx) }
