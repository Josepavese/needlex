package intel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type OllamaRuntime struct {
	BaseURL string
	Client  *http.Client
	Models  RuntimeModels
}

type ollamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []ollamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
	Think    bool                `json:"think"`
	Format   string              `json:"format,omitempty"`
	Options  map[string]any      `json:"options,omitempty"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	DoneReason      string `json:"done_reason"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
}

func (r OllamaRuntime) Run(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if err := req.Validate(); err != nil {
		return ModelResponse{}, err
	}
	model := r.modelForClass(req.ModelClass)
	if model == "" {
		return ModelResponse{}, fmt.Errorf("no model configured for class %q", req.ModelClass)
	}
	body, err := marshalOllamaRequest(model, req)
	if err != nil {
		return ModelResponse{}, err
	}

	httpReq, err := r.buildRequest(ctx, body)
	if err != nil {
		return ModelResponse{}, err
	}
	httpClient := r.httpClient(req.TimeoutMS)
	started := time.Now()
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return ModelResponse{}, err
	}
	defer httpResp.Body.Close() //nolint:errcheck
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return ModelResponse{}, fmt.Errorf("ollama backend returned status %d", httpResp.StatusCode)
	}

	var payload ollamaChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&payload); err != nil {
		return ModelResponse{}, err
	}
	content := strings.TrimSpace(payload.Message.Content)
	if content == "" {
		return ModelResponse{}, fmt.Errorf("ollama backend returned empty content")
	}
	finishReason := strings.TrimSpace(payload.DoneReason)
	if finishReason == "" {
		finishReason = ModelFinishReasonStop
	}
	resp := ModelResponse{
		Model:        model,
		Task:         req.Task,
		OutputJSON:   content,
		TokensIn:     payload.PromptEvalCount,
		TokensOut:    payload.EvalCount,
		LatencyMS:    time.Since(started).Milliseconds(),
		FinishReason: finishReason,
		Confidence:   extractConfidence(content),
	}
	if err := resp.Validate(); err != nil {
		return ModelResponse{}, err
	}
	return resp, nil
}

func marshalOllamaRequest(model string, req ModelRequest) ([]byte, error) {
	messages := promptForRequest(req)
	ollamaMessages := make([]ollamaChatMessage, 0, len(messages))
	for _, item := range messages {
		ollamaMessages = append(ollamaMessages, ollamaChatMessage(item))
	}
	options := map[string]any{
		"temperature": 0,
	}
	if req.MaxOutputTokens > 0 {
		options["num_predict"] = req.MaxOutputTokens
	}
	body := ollamaChatRequest{
		Model:    model,
		Messages: ollamaMessages,
		Stream:   false,
		Think:    false,
		Format:   "json",
		Options:  options,
	}
	return json.Marshal(body)
}

func (r OllamaRuntime) httpClient(timeoutMS int64) *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
}

func (r OllamaRuntime) buildRequest(ctx context.Context, body []byte) (*http.Request, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(r.BaseURL), "/") + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	return httpReq, nil
}

func (r OllamaRuntime) modelForClass(class string) string {
	switch class {
	case ModelClassMicroSolver:
		return strings.TrimSpace(r.Models.MicroSolver)
	default:
		return ""
	}
}
