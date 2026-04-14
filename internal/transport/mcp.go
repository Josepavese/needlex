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

	"github.com/josepavese/needlex/internal/buildinfo"
	"github.com/josepavese/needlex/internal/platform"
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
		fmt.Fprintln(stderr, "mcp stdin is not configured")
		return 1
	}
	logf, closeLog, err := openMCPLog()
	if err != nil {
		fmt.Fprintf(stderr, "mcp log setup failed: %v\n", err)
		return 1
	}
	defer closeLog()

	r.storeRoot = resolveMCPStoreRoot(r.storeRoot)
	if err := os.MkdirAll(r.storeRoot, 0o755); err != nil {
		fmt.Fprintf(stderr, "mcp state root setup failed: %v\n", err)
		return 1
	}
	conn := newMCPConn(r.stdin, stdout)
	for {
		payload, err := conn.ReadPayload()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			mcpLogf(logf, "read failed: %v", err)
			return 1
		}

		var req mcpRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			if err := conn.WriteResponse(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "invalid json"}}); err != nil {
				mcpLogf(logf, "write parse error failed: %v", err)
				return 1
			}
			continue
		}

		resp, respond := r.handleMCP(req)
		if !respond {
			continue
		}
		if err := conn.WriteResponse(resp); err != nil {
			mcpLogf(logf, "write failed: %v", err)
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

func openMCPLog() (io.Writer, func(), error) {
	path := strings.TrimSpace(os.Getenv("NEEDLEX_MCP_LOG"))
	if path == "" {
		path = filepath.Join(os.TempDir(), "needlex-mcp.log")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, nil, err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return file, func() { _ = file.Close() }, nil
}

func mcpLogf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "%s %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
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
			resp.Error = &mcpError{Code: -32000, Message: err.Error()}
			return resp, true
		}
		resp.Result = result
		return resp, true
	default:
		resp.Error = &mcpError{Code: -32601, Message: "method not found"}
		return resp, true
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
		"  NEEDLEX_HOME      override state root",
		"  NEEDLEX_MCP_LOG   override MCP log path (default: $TMPDIR/needlex-mcp.log)",
		"",
		"Notes:",
		"  run without extra positional arguments",
		"  safe to probe with 'needlex mcp --help' before connecting a client",
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
