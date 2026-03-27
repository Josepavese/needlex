package pipeline

import "strings"

type Segmenter struct {
	MaxSegmentChars int
}

func (s Segmenter) Segment(dom SimplifiedDOM) []Segment {
	limit := s.MaxSegmentChars
	if limit <= 0 {
		limit = 1200
	}

	headingPath := []string{}
	segments := make([]Segment, 0, len(dom.Nodes))

	for _, node := range dom.Nodes {
		if node.Kind == "heading" {
			headingPath = nextHeadingPath(headingPath, node.HeadingLevel, node.Text)
			continue
		}

		targetPath := headingPath
		if len(targetPath) == 0 && strings.TrimSpace(dom.Title) != "" {
			targetPath = []string{dom.Title}
		}

		if len(segments) == 0 || !canAppend(segments[len(segments)-1], node, targetPath, limit) {
			segments = append(segments, Segment{
				Kind:        node.Kind,
				HeadingPath: append([]string{}, targetPath...),
				Text:        node.Text,
				NodePaths:   []string{node.Path},
			})
			continue
		}

		current := &segments[len(segments)-1]
		current.Text += "\n\n" + node.Text
		current.NodePaths = append(current.NodePaths, node.Path)
	}

	return segments
}

func nextHeadingPath(current []string, level int, text string) []string {
	if level <= 0 {
		return current
	}
	next := append([]string{}, current...)
	if level-1 < len(next) {
		next = next[:level-1]
	}
	next = append(next, text)
	return next
}

func canAppend(segment Segment, node SimplifiedNode, headingPath []string, limit int) bool {
	if segment.Kind != node.Kind {
		return false
	}
	if strings.Join(segment.HeadingPath, "\x00") != strings.Join(headingPath, "\x00") {
		return false
	}
	return len(segment.Text)+2+len(node.Text) <= limit
}
