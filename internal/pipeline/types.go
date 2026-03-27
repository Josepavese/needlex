package pipeline

import "time"

type AcquireInput struct {
	URL       string
	Timeout   time.Duration
	MaxBytes  int64
	UserAgent string
}

type RawPage struct {
	URL         string
	FinalURL    string
	StatusCode  int
	ContentType string
	HTML        string
	FetchMode   string
	FetchedAt   time.Time
}

type SimplifiedNode struct {
	Path         string
	Tag          string
	Kind         string
	Text         string
	Depth        int
	HeadingLevel int
}

type SimplifiedDOM struct {
	URL   string
	Title string
	Nodes []SimplifiedNode
}

type Segment struct {
	Kind        string
	HeadingPath []string
	Text        string
	NodePaths   []string
}
