// Package document extracts page-aware text from non-HTML documents. PDF is the
// only format adopted by document.read.v1; OCR is deliberately out of scope.
package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/pipeline"
	pdf "github.com/ledongthuc/pdf"
)

// Code is a stable document failure reason. The public contract exposes these
// values instead of collapsing every unusable PDF into unsupported content type.
type Code string

const (
	CodeEncrypted   Code = "document_encrypted"
	CodeMalformed   Code = "document_malformed"
	CodeNoText      Code = "document_no_text"
	CodeUnsupported Code = "document_unsupported"
	CodeLimit       Code = "document_limit_exceeded"
	CodeTimeout     Code = "document_timeout"
)

// Error carries a stable document failure code through the normal Go error
// chain so CLI/MCP classification does not depend on provider wording.
type Error struct {
	Code Code
	Page int
	Err  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	message := string(e.Code)
	if e.Page > 0 {
		message += fmt.Sprintf(" (page %d)", e.Page)
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *Error) Unwrap() error { return e.Err }

// FailureClass lets the existing failure classifier recognize document errors
// without importing this package.
func (e *Error) FailureClass() string { return string(e.Code) }

// ErrorCode returns the document code carried by err, or empty for unrelated
// errors.
func ErrorCode(err error) Code {
	var documentError *Error
	if errors.As(err, &documentError) {
		return documentError.Code
	}
	return ""
}

// Extract reads an already bounded PDF body and maps it onto the shared
// page-aware SimplifiedDOM surface. The caller owns network policy, byte limits,
// source resolution and proof storage.
func Extract(ctx context.Context, page pipeline.RawPage, limits pipeline.DocumentLimits) (pipeline.SimplifiedDOM, error) {
	if err := ctx.Err(); err != nil {
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeTimeout, Err: err}
	}
	timeout := time.Duration(limits.TimeoutMS) * time.Millisecond
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	type result struct {
		dom pipeline.SimplifiedDOM
		err error
	}
	done := make(chan result, 1)
	go func() {
		dom, err := extract(ctx, page, limits)
		done <- result{dom: dom, err: err}
	}()
	select {
	case result := <-done:
		return result.dom, result.err
	case <-ctx.Done():
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeTimeout, Err: ctx.Err()}
	}
}

func extract(ctx context.Context, page pipeline.RawPage, limits pipeline.DocumentLimits) (pipeline.SimplifiedDOM, error) {
	if !pipeline.IsDocumentRawPage(page) || len(page.Body) == 0 {
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeMalformed, Err: errors.New("document body is empty or not a PDF")}
	}
	if limits.MaxBytes <= 0 || limits.MaxPages <= 0 || limits.MaxOutputChars <= 0 || limits.TimeoutMS <= 0 {
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeUnsupported, Err: errors.New("document limits are not configured")}
	}
	if int64(len(page.Body)) > limits.MaxBytes {
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeLimit, Err: fmt.Errorf("document body exceeds %d bytes", limits.MaxBytes)}
	}
	if err := ctx.Err(); err != nil {
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeTimeout, Err: err}
	}

	reader, err := openReader(page.Body)
	if err != nil {
		return pipeline.SimplifiedDOM{}, classifyOpenError(err)
	}
	pagesTotal, err := pageCount(reader)
	if err != nil {
		return pipeline.SimplifiedDOM{}, classifyOpenError(err)
	}
	if pagesTotal == 0 {
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeNoText, Err: errors.New("PDF has no pages")}
	}

	info := &pipeline.DocumentInfo{
		Format:      "pdf",
		RawSHA256:   rawSHA256(page.Body),
		SourceBytes: int64(len(page.Body)),
		PagesTotal:  pagesTotal,
		Coverage:    "complete",
		Limits:      limits,
	}
	dom := pipeline.SimplifiedDOM{
		URL:            page.FinalURL,
		SubstrateClass: "pdf_text",
		SourceKind:     "pdf_text",
		Document:       info,
	}

	var title string
	outputChars := 0
	var firstErr *Error
	processed := 0
	pageLimit := min(pagesTotal, limits.MaxPages)
	if pageLimit < pagesTotal {
		info.Truncated = true
	}
	for pageNumber := 1; pageNumber <= pageLimit; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return pipeline.SimplifiedDOM{}, &Error{Code: CodeTimeout, Page: pageNumber, Err: err}
		}
		processed = pageNumber
		text, pageErr := pageText(reader, pageNumber)
		if pageErr != nil {
			failure := *classifyPageError(pageErr, pageNumber)
			info.PagesFailed = append(info.PagesFailed, pipeline.DocumentPageFailure{
				Page:    pageNumber,
				Code:    string(failure.Code),
				Message: failure.Error(),
			})
			if firstErr == nil {
				firstErr = &failure
			}
			continue
		}
		text = normalizePageText(text)
		if text == "" {
			info.PagesFailed = append(info.PagesFailed, pipeline.DocumentPageFailure{
				Page:    pageNumber,
				Code:    string(CodeNoText),
				Message: "page has no embedded text",
			})
			continue
		}
		info.PagesWithText++
		if title == "" {
			title = pageTitle(text)
		}
		blocks, blockErr := pageBlocks(text, limits.MaxOutputChars-outputChars)
		if blockErr != nil {
			info.Truncated = true
		}
		for blockIndex, block := range blocks {
			if outputChars >= limits.MaxOutputChars {
				info.Truncated = true
				break
			}
			remaining := limits.MaxOutputChars - outputChars
			block = truncateRunes(block, remaining)
			if strings.TrimSpace(block) == "" {
				continue
			}
			dom.Nodes = append(dom.Nodes, pipeline.SimplifiedNode{
				Path:  fmt.Sprintf("/pdf/page[%d]/block[%d]", pageNumber, blockIndex+1),
				Tag:   "p",
				Kind:  "paragraph",
				Text:  block,
				Depth: 2,
				Page:  pageNumber,
			})
			outputChars += utf8Len(block)
		}
		if outputChars >= limits.MaxOutputChars {
			info.Truncated = true
			break
		}
	}

	info.PagesProcessed = processed
	info.ExtractedChars = outputChars
	dom.Title = title
	if info.Truncated || len(info.PagesFailed) > 0 {
		info.Coverage = "partial"
	}
	if len(dom.Nodes) == 0 {
		if firstErr != nil {
			return pipeline.SimplifiedDOM{}, firstErr
		}
		return pipeline.SimplifiedDOM{}, &Error{Code: CodeNoText, Err: errors.New("PDF contains no extractable text; OCR is not supported")}
	}
	return dom, nil
}

func openReader(data []byte) (reader *pdf.Reader, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			reader, err = nil, fmt.Errorf("PDF parser panic: %v", recovered)
		}
	}()
	return pdf.NewReader(bytes.NewReader(data), int64(len(data)))
}

func pageCount(reader *pdf.Reader) (count int, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			count, err = 0, fmt.Errorf("PDF page count panic: %v", recovered)
		}
	}()
	return reader.NumPage(), nil
}

func pageText(reader *pdf.Reader, pageNumber int) (text string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			text, err = "", fmt.Errorf("PDF page parser panic: %v", recovered)
		}
	}()
	page := reader.Page(pageNumber)
	text, err = page.GetPlainText(nil)
	return text, err
}

func classifyOpenError(err error) *Error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "encrypt") || strings.Contains(message, "password") {
		return &Error{Code: CodeEncrypted, Err: err}
	}
	if strings.Contains(message, "unsupported") || strings.Contains(message, "not supported") {
		return &Error{Code: CodeUnsupported, Err: err}
	}
	return &Error{Code: CodeMalformed, Err: err}
}

func classifyPageError(err error, page int) *Error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "encrypt") || strings.Contains(message, "password") {
		return &Error{Code: CodeEncrypted, Page: page, Err: err}
	}
	if strings.Contains(message, "unsupported") || strings.Contains(message, "not supported") {
		return &Error{Code: CodeUnsupported, Page: page, Err: err}
	}
	return &Error{Code: CodeMalformed, Page: page, Err: err}
}

func rawSHA256(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func pageTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
