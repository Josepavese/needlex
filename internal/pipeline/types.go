package pipeline

import "time"

type AcquireInput struct {
	URL                 string
	Timeout             time.Duration
	MaxBytes            int64
	UserAgent           string
	Accept              string
	Profile             string
	RetryProfile        string
	BlockedRetryBackoff time.Duration
	BlockedRetryJitter  time.Duration
	PerHostMinGap       time.Duration
	PerHostJitter       time.Duration
	TimeoutRetryBackoff time.Duration
	TimeoutRetryJitter  time.Duration
	AllowPartial        bool
}

type RawPage struct {
	URL          string
	FinalURL     string
	StatusCode   int
	ContentType  string
	Headers      map[string][]string
	HTML         string
	Partial      bool
	FetchMode    string
	FetchProfile string
	SourceKind   string
	SourceReason string
	SourceFrom   string
	RetryCount   int
	RetryReason  string
	RetrySleepMS int64
	HostPacingMS int64
	FetchedAt    time.Time
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
	URL            string
	Title          string
	SubstrateClass string
	SourceKind     string
	Nodes          []SimplifiedNode
}

type Segment struct {
	Kind        string
	HeadingPath []string
	Text        string
	NodePaths   []string
}
