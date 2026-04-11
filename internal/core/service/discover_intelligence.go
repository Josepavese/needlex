package service

import (
	"context"
	"fmt"
	"strings"

	discoverycore "github.com/josepavese/needlex/internal/core/discovery"
	"github.com/josepavese/needlex/internal/intel"
)

const (
	candidateClassFirstPartyDocs  = "first_party_docs"
	candidateClassFirstPartyHome  = "first_party_home"
	candidateClassThirdPartyGuide = "third_party_tutorial"
	candidateClassThirdPartyWrap  = "third_party_wrapper"
	candidateClassReferenceIndex  = "reference_index"
	candidateClassStructuredAPI   = "structured_endpoint"
	candidateClassAssetFile       = "asset_file"
	candidateClassUnknown         = "unknown"
)

type candidateClassSpec struct {
	Name      string
	Prototype string
	Boost     float64
}

type candidateIntelligence struct {
	Class            string
	ClassConfidence  float64
	ClusterID        string
	ClusterSize      int
	ClusterDominant  string
	ClusterCoherence float64
}

type candidateCluster struct {
	ID           string
	MemberIdx    []int
	Dominant     string
	Coherence    float64
	AvgClassConf float64
	Centrality   map[int]float64
}

var candidateClassSpecs = []candidateClassSpec{
	{Name: candidateClassFirstPartyDocs, Prototype: "official documentation reference developer guide api docs", Boost: 0.34},
	{Name: candidateClassFirstPartyHome, Prototype: "official homepage main site organization home", Boost: 0.20},
	{Name: candidateClassReferenceIndex, Prototype: "reference resources index portal directory catalog", Boost: -0.05},
	{Name: candidateClassStructuredAPI, Prototype: "structured api endpoint schema json xml feed", Boost: 0.10},
	{Name: candidateClassThirdPartyGuide, Prototype: "tutorial walkthrough learning article blog guide", Boost: -0.06},
	{Name: candidateClassThirdPartyWrap, Prototype: "wrapper sdk integration compatibility adapter proxy provider", Boost: -0.12},
	{Name: candidateClassAssetFile, Prototype: "asset icon image stylesheet static file media download", Boost: -0.16},
}

func (s *Service) applyCandidateIntelligence(ctx context.Context, goal string, candidates []DiscoverCandidate) []DiscoverCandidate {
	window := candidateIntelligenceWindow(candidates)
	if window < 2 || strings.TrimSpace(goal) == "" {
		return candidates
	}
	annotated := append([]DiscoverCandidate{}, candidates...)
	texts := make([]intel.SemanticCandidate, 0, window)
	for i := 0; i < window; i++ {
		annotated[i].Metadata = discoverycore.MergeMetadata(annotated[i].Metadata, map[string]string{
			"resource_class": discoverycore.FirstNonEmpty(annotated[i].Metadata["resource_class"], discoverycore.ResourceClass(annotated[i].URL)),
		})
		texts = append(texts, intel.SemanticCandidate{
			ID:   annotated[i].URL,
			Text: candidateSemanticText(annotated[i]),
		})
	}
	classMap := s.classifyCandidateSet(ctx, texts)
	if len(classMap) == 0 {
		return candidates
	}
	graph := s.buildCandidateSemanticGraph(ctx, texts, annotated[:window])
	clusters := buildCandidateClusters(annotated[:window], classMap, graph)
	clusterByIdx := map[int]candidateCluster{}
	for _, cluster := range clusters {
		for _, idx := range cluster.MemberIdx {
			clusterByIdx[idx] = cluster
		}
	}
	for i := 0; i < window; i++ {
		intelMeta, ok := classMap[annotated[i].URL]
		if !ok {
			continue
		}
		cluster := clusterByIdx[i]
		boost := classBoost(intelMeta.Class)
		if cluster.ClusterSize() > 1 {
			boost += 0.05 * float64(min(cluster.ClusterSize()-1, 2))
		}
		if cluster.Dominant == intelMeta.Class && cluster.Coherence > 0 {
			boost += minFloat(cluster.Coherence*0.10, 0.12)
		}
		if centrality, ok := cluster.Centrality[i]; ok && centrality > 0 {
			boost += minFloat(centrality*0.18, 0.22)
		}
		if boost != 0 {
			annotated[i].Score += boost
			annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_intelligence")
			if classBoost(intelMeta.Class) != 0 {
				annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_class_"+intelMeta.Class)
			}
			if cluster.ClusterSize() > 1 {
				annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_cluster_support")
			}
			if centrality, ok := cluster.Centrality[i]; ok && centrality > 0 {
				annotated[i].Reason = discoverycore.AppendUniqueReason(annotated[i].Reason, "candidate_graph_centrality")
			}
		}
		annotated[i].Metadata = discoverycore.MergeMetadata(annotated[i].Metadata, map[string]string{
			"candidate_class":            intelMeta.Class,
			"candidate_class_confidence": fmt.Sprintf("%.3f", intelMeta.ClassConfidence),
			"cluster_id":                 cluster.ID,
			"cluster_size":               fmt.Sprintf("%d", cluster.ClusterSize()),
			"cluster_dominant_class":     cluster.Dominant,
			"cluster_coherence":          fmt.Sprintf("%.3f", cluster.Coherence),
			"cluster_centrality":         fmt.Sprintf("%.3f", cluster.Centrality[i]),
		})
	}
	applyClusterRepresentativeSelection(annotated[:window], clusterByIdx, classMap)
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

func (s *Service) classifyCandidateSet(ctx context.Context, candidates []intel.SemanticCandidate) map[string]candidateIntelligence {
	if len(candidates) == 0 {
		return nil
	}
	scoreByCandidate := map[string]map[string]float64{}
	for _, spec := range candidateClassSpecs {
		scored, err := s.semantic.Score(ctx, spec.Prototype, candidates)
		if err != nil || len(scored) == 0 {
			continue
		}
		for _, item := range scored {
			row := scoreByCandidate[item.ID]
			if row == nil {
				row = map[string]float64{}
				scoreByCandidate[item.ID] = row
			}
			row[spec.Name] = item.Similarity
		}
	}
	if len(scoreByCandidate) == 0 {
		return nil
	}
	out := map[string]candidateIntelligence{}
	for _, candidate := range candidates {
		row := scoreByCandidate[candidate.ID]
		if len(row) == 0 {
			continue
		}
		bestClass, bestScore, second := candidateClassUnknown, -1.0, -1.0
		for _, spec := range candidateClassSpecs {
			score := row[spec.Name]
			if score > bestScore {
				second = bestScore
				bestScore = score
				bestClass = spec.Name
			} else if score > second {
				second = score
			}
		}
		conf := bestScore - maxFloat(second, 0)
		if bestScore <= 0 {
			bestClass = candidateClassUnknown
			conf = 0
		}
		out[candidate.ID] = candidateIntelligence{Class: bestClass, ClassConfidence: conf}
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
			weight := similarity*0.62 + structural*0.38
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
	var (
		lr float64
		rl float64
	)
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

func buildCandidateClusters(candidates []DiscoverCandidate, classes map[string]candidateIntelligence, graph candidateSemanticGraph) []candidateCluster {
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
			li, lok := classes[left.URL]
			ri, rok := classes[right.URL]
			if lok && rok && li.Class != candidateClassUnknown && li.Class == ri.Class && li.ClassConfidence >= 0.08 && ri.ClassConfidence >= 0.08 && edgeWeight >= 0.50 {
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
		countByClass := map[string]int{}
		avgConf := 0.0
		centrality := map[int]float64{}
		for _, idx := range idxs {
			if intelMeta, ok := classes[candidates[idx].URL]; ok {
				countByClass[intelMeta.Class]++
				avgConf += intelMeta.ClassConfidence
			}
			for _, other := range idxs {
				if idx == other {
					continue
				}
				centrality[idx] += graph.weights[idx][other]
			}
		}
		dominant := candidateClassUnknown
		dominantCount := -1
		for _, spec := range candidateClassSpecs {
			if countByClass[spec.Name] > dominantCount {
				dominantCount = countByClass[spec.Name]
				dominant = spec.Name
			}
		}
		coherence := 0.0
		if len(idxs) > 0 {
			coherence = avgConf / float64(len(idxs))
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
		out = append(out, candidateCluster{
			ID:           fmt.Sprintf("cluster_%02d", seq),
			MemberIdx:    idxs,
			Dominant:     dominant,
			Coherence:    coherence,
			AvgClassConf: coherence,
			Centrality:   centrality,
		})
		seq++
	}
	return out
}

func applyClusterRepresentativeSelection(candidates []DiscoverCandidate, clusterByIdx map[int]candidateCluster, classes map[string]candidateIntelligence) {
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
		repScore := clusterRepresentativeScore(candidates[idx], cluster, idx, classes)
		for _, memberIdx := range cluster.MemberIdx {
			score := clusterRepresentativeScore(candidates[memberIdx], cluster, memberIdx, classes)
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

func clusterRepresentativeScore(candidate DiscoverCandidate, cluster candidateCluster, idx int, classes map[string]candidateIntelligence) float64 {
	score := candidate.Score
	if centrality, ok := cluster.Centrality[idx]; ok {
		score += centrality * 0.30
	}
	if intelMeta, ok := classes[candidate.URL]; ok {
		switch intelMeta.Class {
		case candidateClassFirstPartyDocs:
			score += 0.16
		case candidateClassFirstPartyHome:
			score += 0.10
		case candidateClassStructuredAPI:
			score += 0.06
		case candidateClassThirdPartyWrap:
			score -= 0.10
		case candidateClassThirdPartyGuide:
			score -= 0.05
		case candidateClassAssetFile:
			score -= 0.18
		}
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
	if candidate.Metadata["resource_class"] == discoverycore.ResourceClassHTMLLike {
		score += 0.05
	}
	return score
}

func (c candidateCluster) ClusterSize() int { return len(c.MemberIdx) }

func classBoost(class string) float64 {
	for _, spec := range candidateClassSpecs {
		if spec.Name == class {
			return spec.Boost
		}
	}
	return 0
}

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
