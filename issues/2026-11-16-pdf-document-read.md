# PDF document reads for `read` and `web_read`

Status: resolved in v0.1.54.
Last reviewed: 2026-11-16 (v0.1.54).
Source: GitHub issue #4, "Leggere PDF pubblici con testo incorporato e prove per pagina".

## Context

An external conversational runtime used Needle-X v0.1.52/v0.1.53 against a
public PDF with selectable text:

```text
https://pdfobject.com/pdf/sample.pdf
HTTP 200, application/pdf, 18,810 bytes
SHA-256 229defbb0cee6f02673a5cde290d0673e75a0dc31cec43989c8ab2a4eca7e1bb
```

`fetch` saved the original bytes, but `read` returned exit 1 with
`class=unsupported_content_type`. The reproduction used
`NEEDLEX_NETWORK_POLICY=public ... --render off`. Before v0.1.54, only HTML-like
content types reached the reduction pipeline; PDF was explicitly absent.

## Required outcome

Implement text extraction for public PDFs with embedded text on both `read` and
MCP `web_read`, without adding a separate MCP tool. The result must expose:

1. excerpt text and page numbers
2. the final URL after redirects
3. proof references that can be verified with `proof`
4. pages processed, truncation and coverage
5. byte, page, time and output limits
6. distinct reasons for no text, encrypted and malformed PDFs
7. the existing network policy and privacy boundary
8. a versioned output contract that an external consumer can adopt

OCR is explicitly out of scope.

## Design

The existing read contract is extended instead of adding another surface:

1. `internal/document` owns static, local, page-aware PDF extraction behind
   `document.read.v1`. It is not a browser, renderer, system helper or OCR path.
2. PDF acquisition keeps the received bytes in `pipeline.RawPage.Body`; the
   normal source resolver and `MaybeRender` stages are skipped for documents.
3. Extraction maps each page's plain text into the shared `SimplifiedNode`
   model with `Page`, `/pdf/page[N]/block[M]` selectors and page-aware segments.
   Segmentation does not merge nodes across pages.
4. `core.DocumentExtraction` is serialized as a compact `document_extraction`
   object in both CLI and MCP, and is included in full diagnostics. It contains
   `raw_sha256`, `source_bytes`, page counts, page failures, `truncated`,
   `coverage` and the effective `limits`.
5. Chunks carry `page_start`/`page_end`; proof source spans carry `page`.
   `signals.content_source`/`substrate_class` are `pdf_text`.
6. `render=required` on a PDF fails with `render_not_applicable`. `render=off`
   and `render=auto` use static extraction, and PDFs never launch a browser.
7. Oversized document bodies are rejected rather than truncated into malformed
   PDF bytes, and map to `document_limit_exceeded`.
8. Stable failure classes are `document_encrypted`, `document_malformed`,
   `document_no_text`, `document_unsupported`, `document_limit_exceeded` and
   `document_timeout`.
9. Limits are configurable under `document.max_pages`,
   `document.max_output_chars` and `document.timeout_ms`; the byte ceiling
   follows `runtime.max_bytes`. Defaults are 100 pages, 200,000 output
   characters and 5,000 ms.
10. The PDF parser is `github.com/ledongthuc/pdf`, a pure-Go BSD-3-Clause
    fork of `rsc.io/pdf` with a page-aware plain-text API. It raises the direct
    runtime dependency count from five to six, recorded with a measured budget
    update. No external process is executed.

## Acceptance Evidence

Unit and service tests cover:

1. exact sample title, first sentence, page tag and full SHA-256
2. encrypted, malformed and blank/no-text fixtures with distinct classes
3. byte limit, page limit, output truncation, partial coverage and timeout
4. strict `render=required` rejection
5. compact CLI and MCP payloads carrying `document_extraction`, page ranges and
   `pdf_text`
6. proof/source-span page propagation

Manual installed-runtime evidence on v0.1.54:

```text
read URL: https://pdfobject.com/pdf/sample.pdf
title: Sample PDF
raw_sha256: 229defbb0cee6f02673a5cde290d0673e75a0dc31cec43989c8ab2a4eca7e1bb
source_bytes: 18810
pages: 1/1 processed, 1 with text, coverage=complete, truncated=false
chunk: /pdf/page[1]/block[1], page_start=1, page_end=1
proof source_span.page: 1
MCP structuredContent: document_extraction present
MCP content.text: document_extraction and page_start present
render=required: exit 1, class=render_not_applicable
```

## Non-Goals And Residual Limits

1. No OCR; image-only PDFs return `document_no_text`.
2. No PDF browser rendering; `render=required` is not applicable.
3. No `pdftotext` or other system process.
4. No form fields, annotations, embedded attachments or layout reconstruction;
   only reading-order text and page identity are promised.
5. No password prompt or key derivation: encrypted files return
   `document_encrypted`.
6. The parser dependency is a deliberate, measured budget change, not a free
   capability. A future replacement must preserve page-aware proof and the
   `document.read.v1` contract.

## Release

Shipped in v0.1.54 with:
1. `schemas/needlex-document-read.v1.json`
2. updated CLI/MCP/packet/skill documentation
3. regenerated `THIRD_PARTY_NOTICES.txt`
4. governance budget measured at 40,336 runtime lines, 33 files over 300 lines,
   23 over 350, six direct dependencies and a 6,894-line service package.
