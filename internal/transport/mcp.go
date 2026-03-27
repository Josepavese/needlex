package transport

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/store"
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

func (r Runner) runMCP(args []string, stdout, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "mcp does not accept positional arguments")
		return 2
	}
	if r.stdin == nil {
		fmt.Fprintln(stderr, "mcp stdin is not configured")
		return 1
	}

	reader := bufio.NewReader(r.stdin)
	for {
		payload, err := readMCPFrame(reader)
		if err != nil {
			if err == io.EOF {
				return 0
			}
			fmt.Fprintf(stderr, "mcp read failed: %v\n", err)
			return 1
		}

		var req mcpRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			if err := writeMCPFrame(stdout, mcpResponse{
				JSONRPC: "2.0",
				Error:   &mcpError{Code: -32700, Message: "invalid json"},
			}); err != nil {
				fmt.Fprintf(stderr, "mcp write failed: %v\n", err)
				return 1
			}
			continue
		}

		resp, respond := r.handleMCP(req)
		if !respond {
			continue
		}
		if err := writeMCPFrame(stdout, resp); err != nil {
			fmt.Fprintf(stderr, "mcp write failed: %v\n", err)
			return 1
		}
	}
}

func (r Runner) handleMCP(req mcpRequest) (mcpResponse, bool) {
	resp := mcpResponse{JSONRPC: "2.0", ID: req.ID}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "needlex",
				"version": "0.1.0",
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

func (r Runner) callMCPTool(call mcpToolCall) (map[string]any, error) {
	switch call.Name {
	case "web_read":
		cfg, err := config.Load(stringArg(call.Arguments, "config_path"))
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		if laneMax, ok := intArg(call.Arguments, "lane_max"); ok {
			cfg.Runtime.LaneMax = laneMax
		}
		resp, artifacts, err := r.executeRead(cfg, coreservice.ReadRequest{
			URL:       stringArg(call.Arguments, "url"),
			Objective: stringArg(call.Arguments, "objective"),
			Profile:   stringArg(call.Arguments, "profile"),
			UserAgent: stringArg(call.Arguments, "user_agent"),
		})
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"document":         resp.Document,
			"chunks":           resp.ResultPack.Chunks,
			"proof_refs":       resp.ResultPack.ProofRefs,
			"cost_report":      resp.ResultPack.CostReport,
			"profile":          resp.ResultPack.Profile,
			"trace_id":         resp.Trace.TraceID,
			"trace_path":       artifacts.TracePath,
			"proof_path":       artifacts.ProofPath,
			"fingerprint_path": artifacts.FingerprintPath,
		}
		return mcpToolResult(payload), nil
	case "web_replay":
		report, err := r.loadReplay(stringArg(call.Arguments, "trace_id"))
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"replay_report": report}), nil
	case "web_diff":
		report, err := r.loadDiff(stringArg(call.Arguments, "trace_a"), stringArg(call.Arguments, "trace_b"))
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"diff_report": report}), nil
	case "web_proof":
		lookup := stringArg(call.Arguments, "chunk_id")
		if lookup == "" {
			lookup = stringArg(call.Arguments, "trace_id")
		}
		result, err := r.loadProof(lookup)
		if err != nil {
			return nil, err
		}
		payload := map[string]any{
			"trace_id":      result.TraceID,
			"proof_records": result.Records,
		}
		if len(result.Records) == 1 {
			payload["proof"] = result.Records[0]
		}
		return mcpToolResult(payload), nil
	case "web_prune":
		pruneAll := boolArg(call.Arguments, "all")
		olderThanHours, _ := intArg(call.Arguments, "older_than_hours")
		report, err := store.Prune(r.storeRoot, durationHours(olderThanHours), pruneAll, timeNowUTC())
		if err != nil {
			return nil, err
		}
		return mcpToolResult(map[string]any{"prune_report": report}), nil
	default:
		return nil, fmt.Errorf("unsupported tool %q", call.Name)
	}
}

func mcpTools() []mcpTool {
	return []mcpTool{
		{
			Name:        "web_read",
			Description: "Read a web page into compact proof-carrying context.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":        map[string]any{"type": "string"},
					"profile":    map[string]any{"type": "string"},
					"objective":  map[string]any{"type": "string"},
					"user_agent": map[string]any{"type": "string"},
					"lane_max":   map[string]any{"type": "integer"},
				},
				"required": []string{"url"},
			},
		},
		{
			Name:        "web_replay",
			Description: "Replay a stored trace and report deterministic stage completion.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"trace_id": map[string]any{"type": "string"},
				},
				"required": []string{"trace_id"},
			},
		},
		{
			Name:        "web_diff",
			Description: "Compare two stored traces and report changed stages.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"trace_a": map[string]any{"type": "string"},
					"trace_b": map[string]any{"type": "string"},
				},
				"required": []string{"trace_a", "trace_b"},
			},
		},
		{
			Name:        "web_proof",
			Description: "Load proof records by trace id or chunk id.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"trace_id": map[string]any{"type": "string"},
					"chunk_id": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:        "web_prune",
			Description: "Prune local traces, proofs, and fingerprints.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"all":              map[string]any{"type": "boolean"},
					"older_than_hours": map[string]any{"type": "integer"},
				},
			},
		},
	}
}

func mcpToolResult(payload map[string]any) map[string]any {
	return map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": mustJSON(payload),
			},
		},
		"structuredContent": payload,
		"isError":           false,
	}
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

func timeNowUTC() time.Time {
	return time.Now().UTC()
}
