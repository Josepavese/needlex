package service

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

func (s *Service) applyCandidateIntelligence(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	window := candidateIntelligenceWindow(candidates)
	if window < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	annotated := append([]DiscoverCandidate{}, candidates...)
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
				annotated[i].Metadata["host_root_title"],
				annotated[i].Metadata["page_title"],
				annotated[i].Label,
			),
		})
		hostTexts = append(hostTexts, intel.SemanticCandidate{
			ID:   annotated[i].URL,
			Text: discoverycore.JoinNonEmpty(annotated[i].Metadata["host_root_title"]),
		})
		pageTexts = append(pageTexts, intel.SemanticCandidate{
			ID: annotated[i].URL,
			Text: discoverycore.JoinNonEmpty(
				annotated[i].Metadata["page_title"],
				annotated[i].Label,
			),
		})
	}

	goalSimilarity := s.scoreCandidateSetToGoal(ctx, goal, texts)
	for url, value := range s.scoreCandidateSetToGoal(ctx, goal, identityTexts) {
		if value > goalSimilarity[url] {
			goalSimilarity[url] = value
		}
	}
	hostSimilarity := s.scoreCandidateSetToGoal(ctx, goal, hostTexts)
	pageSimilarity := s.scoreCandidateSetToGoal(ctx, goal, pageTexts)
	if len(goalSimilarity) == 0 {
		return candidates
	}
	graph := s.buildCandidateSemanticGraph(ctx, texts, annotated[:window])
	clusters := buildCandidateClusters(annotated[:window], goalSimilarity, graph)
	clusterByIdx := map[int]candidateCluster{}
	for _, cluster := range clusters {
		for _, idx := range cluster.MemberIdx {
			clusterByIdx[idx] = cluster
		}
	}

	for i := 0; i < window; i++ {
		cluster := clusterByIdx[i]
		similarity := goalSimilarity[annotated[i].URL]
		hostSim := hostSimilarity[annotated[i].URL]
		pageSim := pageSimilarity[annotated[i].URL]
		boost := minFloat(similarity*0.28, 0.20)
		if cluster.ClusterSize() > 1 {
			boost += 0.04 * float64(min(cluster.ClusterSize()-1, 2))
		}
		if cluster.Coherence > 0 {
			boost += minFloat(cluster.Coherence*0.07, 0.08)
		}
		if centrality, ok := cluster.Centrality[i]; ok && centrality > 0 {
			boost += minFloat(centrality*0.14, 0.16)
		}
		switch annotated[i].Metadata["resource_class"] {
		case discoverycore.ResourceClassHTMLLike:
			boost += 0.03
		case discoverycore.ResourceClassMediaAsset:
			boost -= 0.12
		case discoverycore.ResourceClassArchiveFile:
			boost -= 0.08
		case discoverycore.ResourceClassStructured:
			boost -= 0.02
		}
		if pageSim >= 0.24 && hostSim >= 0.16 {
			boost += 0.12
			annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_identity_alignment")
		}
		if pageSim >= 0.24 && hostSim > 0 && hostSim < 0.10 {
			boost -= 0.18
			annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_identity_mismatch")
		}
		if boost != 0 {
			annotated[i].Score += boost
			annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_intelligence")
			if cluster.ClusterSize() > 1 {
				annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_cluster_support")
			}
			if similarity > 0 {
				annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_goal_grounding")
			}
			if centrality, ok := cluster.Centrality[i]; ok && centrality > 0 {
				annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_graph_centrality")
			}
		}
		annotated[i].Metadata = discoverycore.MergeMetadata(annotated[i].Metadata, map[string]string{
			"candidate_goal_similarity": fmt.Sprintf("%.3f", similarity),
			"candidate_host_similarity": fmt.Sprintf("%.3f", hostSim),
			"candidate_page_similarity": fmt.Sprintf("%.3f", pageSim),
			"cluster_id":                cluster.ID,
			"cluster_size":              fmt.Sprintf("%d", cluster.ClusterSize()),
			"cluster_family":            cluster.Family,
			"cluster_coherence":         fmt.Sprintf("%.3f", cluster.Coherence),
			"cluster_centrality":        fmt.Sprintf("%.3f", cluster.Centrality[i]),
		})
	}
	applyClusterRepresentativeSelection(annotated[:window], clusterByIdx, goalSimilarity)
	discoverycore.SortCandidates(annotated)
	return annotated
}

func candidateIntelligenceWindow(candidates []DiscoverCandidate) int {
	if len(candidates) < 2 {
		return 0
	}
	if candidates[0].Score-candidates[1].Score > 0.32 {
		return 0
	}
	window := 0
	topScore := candidates[0].Score
	for i := 0; i < len(candidates) && i < 5; i++ {
		if topScore-candidates[i].Score > 0.55 {
			break
		}
		window++
	}
	if window < 2 {
		return 0
	}
	return window
}

func candidateSemanticText(candidate DiscoverCandidate) string {
	return discoverycore.JoinNonEmpty(
		candidate.Metadata["host_root_title"],
		candidate.Metadata["page_title"],
		strings.TrimSpace(candidate.Label),
		discoverycore.URLTokenText(candidate.URL),
		candidate.Metadata["resource_class"],
	)
}

func (s *Service) scoreCandidateSetToGoal(ctx context.Context, goal string, candidates []intel.SemanticCandidate) map[string]float64 {
	if len(candidates) == 0 || strings.TrimSpace(goal) == "" {
		return nil
	}
	scored, err := s.semantic.Score(ctx, goal, candidates)
	if err != nil || len(scored) == 0 {
		return nil
	}
	out := make(map[string]float64, len(scored))
	for _, item := range scored {
		out[item.ID] = maxFloat(item.Similarity, 0)
	}
	return out
}

type candidateSemanticGraph struct {
	weights map[int]map[int]float64
}

func (s *Service) buildCandidateSemanticGraph(ctx context.Context, texts []intel.SemanticCandidate, candidates []DiscoverCandidate) candidateSemanticGraph {
	graph := candidateSemanticGraph{weights: map[int]map[int]float64{}}
	if len(texts) < 2 {
		return graph
	}
	raw := make([][]float64, len(texts))
	for i := range texts {
		scores, err := s.semantic.Score(ctx, texts[i].Text, texts)
		if err != nil || len(scores) == 0 {
			continue
		}
		row := make([]float64, len(texts))
		indexByID := map[string]int{}
		for idx, candidate := range texts {
			indexByID[candidate.ID] = idx
		}
		for _, score := range scores {
			if idx, ok := indexByID[score.ID]; ok {
				row[idx] = maxFloat(score.Similarity, 0)
			}
		}
		raw[i] = row
	}
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

func candidateStructuralAffinity(left, right DiscoverCandidate) float64 {
	affinity := 0.0
	if sameCandidateFamily(left.URL, right.URL) {
		affinity = maxFloat(affinity, 1.0)
	}
	if sameDiscoverHost(left.URL, right.URL) {
		affinity = maxFloat(affinity, 0.95)
	}
	if left.Metadata["host_root_title"] != "" && left.Metadata["host_root_title"] == right.Metadata["host_root_title"] {
		affinity = maxFloat(affinity, 0.72)
	}
	if left.Metadata["resource_class"] != "" && left.Metadata["resource_class"] == right.Metadata["resource_class"] {
		affinity = maxFloat(affinity, 0.28)
	}
	return affinity
}

func buildCandidateClusters(candidates []DiscoverCandidate, goalSimilarity map[string]float64, graph candidateSemanticGraph) []candidateCluster {
	if len(candidates) == 0 {
		return nil
	}
	parent := make([]int, len(candidates))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			left, right := candidates[i], candidates[j]
			edgeWeight := graph.weights[i][j]
			if sameCandidateFamily(left.URL, right.URL) || edgeWeight >= 0.68 {
				union(i, j)
				continue
			}
			if goalSimilarity[left.URL] >= 0.28 && goalSimilarity[right.URL] >= 0.28 && edgeWeight >= 0.48 {
				union(i, j)
			}
		}
	}
	groups := map[int][]int{}
	for i := range candidates {
		groups[find(i)] = append(groups[find(i)], i)
	}
	out := make([]candidateCluster, 0, len(groups))
	seq := 1
	for _, idxs := range groups {
		centrality := map[int]float64{}
		sumCoherence := 0.0
		familyCounts := map[string]int{}
		for _, idx := range idxs {
			if family, ok := candidateFamily(candidates[idx].URL); ok {
				familyCounts[family]++
			}
			sumCoherence += goalSimilarity[candidates[idx].URL]
			for _, other := range idxs {
				if idx == other {
					continue
				}
				centrality[idx] += graph.weights[idx][other]
			}
		}
		maxCentrality := 0.0
		for _, value := range centrality {
			maxCentrality = maxFloat(maxCentrality, value)
		}
		if maxCentrality > 0 {
			for idx, value := range centrality {
				centrality[idx] = value / maxCentrality
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
		out = append(out, candidateCluster{
			ID:         fmt.Sprintf("cluster_%02d", seq),
			MemberIdx:  idxs,
			Family:     family,
			Coherence:  sumCoherence / float64(len(idxs)),
			Centrality: centrality,
		})
		seq++
	}
	return out
}

func applyClusterRepresentativeSelection(candidates []DiscoverCandidate, clusterByIdx map[int]candidateCluster, goalSimilarity map[string]float64) {
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

func clusterRepresentativeScore(candidate DiscoverCandidate, cluster candidateCluster, idx int, goalSimilarity map[string]float64) float64 {
	score := candidate.Score
	if centrality, ok := cluster.Centrality[idx]; ok {
		score += centrality * 0.30
	}
	if similarity := goalSimilarity[candidate.URL]; similarity > 0 {
		score += minFloat(similarity*0.24, 0.18)
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

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func minFloat(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}
