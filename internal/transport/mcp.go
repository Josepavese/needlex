package transport

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/observability"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/platform/buildinfo"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *mcpError `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type mcpFramingMode string

const (
	mcpFramingUnknown mcpFramingMode = "unknown"
	mcpFramingLSP     mcpFramingMode = "content-length"
	mcpFramingRaw     mcpFramingMode = "raw-json"
)

type mcpConn struct {
	reader  *bufio.Reader
	writer  io.Writer
	decoder *json.Decoder
	mode    mcpFramingMode
}

func newMCPConn(stdin io.Reader, stdout io.Writer) *mcpConn {
	return &mcpConn{reader: bufio.NewReader(stdin), writer: stdout, mode: mcpFramingUnknown}
}

func (c *mcpConn) ReadPayload() ([]byte, error) {
	if c.mode == mcpFramingUnknown {
		mode, err := detectMCPFraming(c.reader)
		if err != nil {
			return nil, err
		}
		c.mode = mode
		if mode == mcpFramingRaw {
			c.decoder = json.NewDecoder(c.reader)
			c.decoder.UseNumber()
		}
	}
	if c.mode == mcpFramingLSP {
		return readMCPFrame(c.reader)
	}
	var raw json.RawMessage
	if err := c.decoder.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *mcpConn) WriteResponse(value any) error {
	if c.mode == mcpFramingRaw {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := c.writer.Write(data); err != nil {
			return err
		}
		_, err = c.writer.Write([]byte("\n"))
		return err
	}
	return writeMCPFrame(c.writer, value)
}

func (r Runner) runMCP(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 {
		switch strings.TrimSpace(args[0]) {
		case "-h", "--help", "help":
			writeMCPUsage(stdout)
			return 0
		}
	}
	if len(args) != 0 {
		fmt.Fprintln(stderr, "mcp does not accept positional arguments")
		writeMCPUsage(stderr)
		return 2
	}
	if r.stdin == nil {
		return r.reportCLIError(stderr, "mcp", fmt.Errorf("mcp stdin is not configured"), nil)
	}
	r.storeRoot = resolveMCPStoreRoot(r.storeRoot)
	if err := os.MkdirAll(r.storeRoot, 0o755); err != nil {
		return r.reportCLIError(stderr, "mcp", err, map[string]any{"phase": "state_root_setup", "state_root": r.storeRoot})
	}
	conn := newMCPConn(r.stdin, stdout)
	for {
		payload, err := conn.ReadPayload()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			r.logMCPError("mcp_read", "mcp.read_failed", err, nil)
			return 1
		}

		var req mcpRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			r.logMCPError("mcp_parse", "mcp.invalid_json", err, map[string]any{"payload_bytes": len(payload)})
			if err := conn.WriteResponse(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "invalid json"}}); err != nil {
				r.logMCPError("mcp_write", "mcp.write_failed", err, map[string]any{"after": "invalid_json"})
				return 1
			}
			continue
		}

		resp, respond := r.handleMCP(req)
		if !respond {
			continue
		}
		if err := conn.WriteResponse(resp); err != nil {
			r.logMCPError("mcp_write", "mcp.write_failed", err, map[string]any{"method": req.Method})
			return 1
		}
	}
}

func resolveMCPStoreRoot(root string) string {
	root = strings.TrimSpace(root)
	if root != "" && filepath.IsAbs(root) {
		return root
	}
	return platform.StableStateRoot()
}

func (r Runner) handleMCP(req mcpRequest) (mcpResponse, bool) {
	resp := mcpResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "needlex",
				"version": buildinfo.Version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}
		return resp, true
	case "notifications/initialized":
		return mcpResponse{}, false
	case "tools/list":
		resp.Result = map[string]any{"tools": mcpTools()}
		return resp, true
	case "tools/call":
		var call mcpToolCall
		if err := json.Unmarshal(req.Params, &call); err != nil {
			resp.Error = &mcpError{Code: -32602, Message: "invalid tools/call params"}
			return resp, true
		}
		result, err := r.callMCPTool(call)
		if err != nil {
			event := r.logMCPError(call.Name, "mcp.tool_failed", err, map[string]any{"tool": call.Name})
			resp.Error = &mcpError{Code: -32000, Message: mcpToolErrorMessage(err)}
			if event.ID != "" {
				resp.Error.Message += "; diagnostic_id: " + event.ID
			}
			return resp, true
		}
		resp.Result = result
		return resp, true
	default:
		resp.Error = &mcpError{Code: -32601, Message: "method not found"}
		return resp, true
	}
}

func mcpToolErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := observability.RedactString(err.Error())
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "lane_max"):
		return `lane_max is no longer supported; use retrieval_effort instead. Valid retrieval_effort values: minimal, light, balanced, standard, exhaustive`
	case strings.Contains(lower, "quality_budget"):
		return `quality_budget is not supported; use retrieval_effort instead. Valid retrieval_effort values: minimal, light, balanced, standard, exhaustive`
	case strings.Contains(lower, "discovery_mode=off") || strings.Contains(lower, "unexpected status code 404"):
		return message + `; next_recommended_call: web_query with discovery_mode="same_site_links" and the same seed_url, or discovery_mode="web_search" if the seed URL is uncertain`
	case strings.Contains(lower, "unsupported discovery_mode"):
		return message + `; valid discovery_mode values: same_site_links, web_search, off`
	case strings.Contains(lower, "unsupported content type"):
		return message + `; next_recommended_call: use web_query to find an HTML/text equivalent, or use a browser/full-fetch tool when exact binary layout is required`
	case strings.Contains(lower, "provider blocked") || strings.Contains(lower, "anti-bot") || strings.Contains(lower, "status code 429") || strings.Contains(lower, "status code 403"):
		return message + `; next_recommended_call: retry later, use a verified seed_url with same_site_links, or configure a healthier discovery provider`
	default:
		return message
	}
}

func detectMCPFraming(reader *bufio.Reader) (mcpFramingMode, error) {
	for {
		peek, err := reader.Peek(1)
		if err != nil {
			return mcpFramingUnknown, err
		}
		if !isMCPWhitespace(peek[0]) {
			break
		}
		if _, err := reader.ReadByte(); err != nil {
			return mcpFramingUnknown, err
		}
	}
	peek, err := reader.Peek(32)
	if err != nil && !errors.Is(err, bufio.ErrBufferFull) && !errors.Is(err, io.EOF) {
		return mcpFramingUnknown, err
	}
	lower := strings.ToLower(string(peek))
	if strings.HasPrefix(lower, "content-length:") {
		return mcpFramingLSP, nil
	}
	return mcpFramingRaw, nil
}

func isMCPWhitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func mcpToolResult(payload map[string]any, display any) map[string]any {
	if display == nil {
		display = payload
	}
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": mustJSON(display)}},
		"structuredContent": payload,
		"isError":           false,
	}
}

func writeMCPUsage(w io.Writer) {
	writeUsage(
		w,
		"needlex mcp",
		"stdio MCP server for AI clients and MCP hosts",
		"",
		"Behavior:",
		"  accepts both Content-Length framing and raw newline-delimited JSON-RPC",
		"  replies using the same framing mode detected from the client",
		"",
		"Environment:",
		"  NEEDLEX_HOME      override PAL state root",
		"",
		"Notes:",
		"  run without extra positional arguments",
		"  safe to probe with 'needlex mcp --help' before connecting a client",
		"  diagnostics are written to the PAL runtime log; inspect with 'needlex logs path' and 'needlex logs tail'",
	)
}

func readMCPFrame(reader *bufio.Reader) ([]byte, error) {
	contentLength := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		if line == "\r\n" {
			break
		}
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.EqualFold(name, "Content-Length") {
			contentLength, err = strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("invalid content length: %w", err)
			}
		}
	}
	if contentLength <= 0 {
		return nil, fmt.Errorf("missing content length")
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeMCPFrame(w io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

func intArg(args map[string]any, key string) (int, bool) {
	value, ok := args[key]
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return int(typed), true
	case int:
		return typed, true
	case json.Number:
		n, err := typed.Int64()
		if err == nil {
			return int(n), true
		}
	}
	return 0, false
}

func boolArg(args map[string]any, key string) bool {
	value, ok := args[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func durationHours(hours int) time.Duration {
	if hours <= 0 {
		return 0
	}
	return time.Duration(hours) * time.Hour
}

func timeNowUTC() time.Time { return time.Now().UTC() }
