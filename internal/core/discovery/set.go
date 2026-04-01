package discovery

import "strings"

type Set struct {
	items []Candidate
	index map[string]int
}

func NewSet(candidates []Candidate) Set {
	set := Set{items: make([]Candidate, 0, len(candidates)), index: map[string]int{}}
	set.Merge(candidates)
	return set
}

func (s *Set) Merge(candidates []Candidate) {
	for _, candidate := range candidates {
		s.Add(candidate)
	}
}

func (s *Set) Add(candidate Candidate) {
	if strings.TrimSpace(candidate.URL) == "" {
		return
	}
	if idx, ok := s.index[candidate.URL]; ok {
		current := &s.items[idx]
		if candidate.Score > current.Score {
			current.Score = candidate.Score
		}
		current.Label = FirstNonEmpty(candidate.Label, current.Label)
		current.Reason = AppendUniqueReason(current.Reason, candidate.Reason...)
		current.Metadata = MergeMetadata(current.Metadata, candidate.Metadata)
		return
	}
	s.index[candidate.URL] = len(s.items)
	s.items = append(s.items, candidate)
}

func (s Set) Sorted() []Candidate {
	out := append([]Candidate{}, s.items...)
	SortCandidates(out)
	return out
}

func (s Set) Limited(maxCandidates int) []Candidate {
	out := s.Sorted()
	if maxCandidates <= 0 || len(out) <= maxCandidates {
		return out
	}
	return append([]Candidate{}, out[:maxCandidates]...)
}

func (s Set) SelectedURL(fallback string) string {
	if len(s.items) == 0 {
		return fallback
	}
	return s.Sorted()[0].URL
}

func (s Set) URLs() []string {
	out := make([]string, 0, len(s.items))
	for _, candidate := range s.items {
		out = append(out, candidate.URL)
	}
	return out
}

func (s Set) ByURL(selectedURL string) Candidate {
	if selectedURL == "" && len(s.items) > 0 {
		return s.Sorted()[0]
	}
	if idx, ok := s.index[selectedURL]; ok {
		return s.items[idx]
	}
	return Candidate{}
}

func MergeMetadata(existing, incoming map[string]string) map[string]string {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range existing {
		out[key] = value
	}
	for key, value := range incoming {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[key] = value
	}
	return out
}
