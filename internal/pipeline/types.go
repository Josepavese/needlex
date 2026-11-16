package pipeline

import (
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/platform"
)

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
	// Session carries caller-provided navigation credentials. Nil preserves the
	// historical unauthenticated transport.
	Session *platform.Session
}

type RawPage struct {
	URL         string
	FinalURL    string
	StatusCode  int
	ContentType string
	Headers     map[string][]string
	HTML        string
	// Body preserves the undecoded response bytes for non-HTML documents such as
	// PDF, where the text pipeline must parse bytes instead of treating them as
	// an HTML/text payload. HTML pages keep using HTML.
	Body             []byte
	Partial          bool
	FetchMode        string
	FetchProfile     string
	SourceKind       string
	SourceReason     string
	SourceFrom       string
	NetworkText      string
	NetworkBytes     int64
	NetworkResources int
	NetworkTruncated bool
	RetryCount       int
	RetryReason      string
	RetrySleepMS     int64
	HostPacingMS     int64
	FetchedAt        time.Time
}

func IsHTMLLikeRawPage(page RawPage) bool {
	contentType := strings.ToLower(strings.TrimSpace(page.ContentType))
	return contentType == "" || strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml+xml")
}

// IsDocumentRawPage reports document formats that are parsed into a page-aware
// text surface instead of the HTML/text reducer. PDF is the only supported
// format in the document contract v1.
func IsDocumentRawPage(page RawPage) bool {
	contentType := strings.ToLower(strings.TrimSpace(page.ContentType))
	return strings.Contains(contentType, "application/pdf") || strings.Contains(contentType, "application/x-pdf")
}

type SimplifiedNode struct {
	Path         string
	Tag          string
	Kind         string
	Text         string
	Depth        int
	HeadingLevel int
	// Page is the 1-based source page for document formats. Zero means the node
	// has no page identity (ordinary HTML/text).
	Page int
}

type SimplifiedDOM struct {
	URL            string
	Title          string
	SubstrateClass string
	SourceKind     string
	Nodes          []SimplifiedNode
	Document       *DocumentInfo
}

// DocumentInfo is the internal page-aware extraction surface. It is converted
// to core.DocumentExtraction before crossing the public service boundary.
type DocumentInfo struct {
	Format         string
	RawSHA256      string
	SourceBytes    int64
	PagesTotal     int
	PagesProcessed int
	PagesWithText  int
	PagesFailed    []DocumentPageFailure
	Truncated      bool
	Coverage       string
	ExtractedChars int
	Limits         DocumentLimits
}

type DocumentPageFailure struct {
	Page    int
	Code    string
	Message string
}

type DocumentLimits struct {
	MaxBytes       int64
	MaxPages       int
	MaxOutputChars int
	TimeoutMS      int64
}

type Segment struct {
	Kind        string
	HeadingPath []string
	Text        string
	NodePaths   []string
	PageStart   int
	PageEnd     int
}
