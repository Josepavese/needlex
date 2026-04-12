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

type RuntimeModels struct {
	MicroSolver string
}

type OpenAICompatibleRuntime struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
	Models  RuntimeModels
}

type openAIChatRequest struct {
	Model          string              `json:"model"`
	Messages       []openAIChatMessage `json:"messages"`
	Temperature    float64             `json:"temperature"`
	MaxTokens      int                 `json:"max_tokens,omitempty"`
	ResponseFormat map[string]string   `json:"response_format,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (r OpenAICompatibleRuntime) Run(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if err := req.Validate(); err != nil {
		return ModelResponse{}, err
	}
	model := r.modelForClass(req.ModelClass)
	if model == "" {
		return ModelResponse{}, fmt.Errorf("no model configured for class %q", req.ModelClass)
	}
	body, err := marshalChatRequest(model, req)
	if err != nil {
		return ModelResponse{}, err
	}

	httpReq, err := r.buildRequest(ctx, body, req.TimeoutMS)
	if err != nil {
		return ModelResponse{}, err
	}
	httpClient := r.httpClient(req.TimeoutMS)
	started := time.Now()
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return ModelResponse{}, err
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return ModelResponse{}, fmt.Errorf("model backend returned status %d", httpResp.StatusCode)
	}

	payload, err := decodeChatResponse(httpResp)
	if err != nil {
		return ModelResponse{}, err
	}
	resp := buildModelResponse(model, req.Task, payload, time.Since(started).Milliseconds())
	if err := resp.Validate(); err != nil {
		return ModelResponse{}, err
	}
	return resp, nil
}

func marshalChatRequest(model string, req ModelRequest) ([]byte, error) {
	return json.Marshal(openAIChatRequest{
		Model:          model,
		Messages:       promptForRequest(req),
		Temperature:    0,
		MaxTokens:      req.MaxOutputTokens,
		ResponseFormat: map[string]string{"type": "json_object"},
	})
}

func (r OpenAICompatibleRuntime) httpClient(timeoutMS int64) *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
}

func (r OpenAICompatibleRuntime) buildRequest(ctx context.Context, body []byte, timeoutMS int64) (*http.Request, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(r.BaseURL), "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(r.APIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(r.APIKey))
	}
	_ = timeoutMS
	return httpReq, nil
}

func decodeChatResponse(httpResp *http.Response) (openAIChatResponse, error) {
	var payload openAIChatResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&payload); err != nil {
		return openAIChatResponse{}, err
	}
	if len(payload.Choices) == 0 {
		return openAIChatResponse{}, fmt.Errorf("model backend returned no choices")
	}
	return payload, nil
}

func buildModelResponse(model, task string, payload openAIChatResponse, latencyMS int64) ModelResponse {
	content := strings.TrimSpace(payload.Choices[0].Message.Content)
	finishReason := strings.TrimSpace(payload.Choices[0].FinishReason)
	switch finishReason {
	case "", "length":
		finishReason = ModelFinishReasonStop
	}
	return ModelResponse{
		Model:        model,
		Task:         task,
		OutputJSON:   content,
		TokensIn:     payload.Usage.PromptTokens,
		TokensOut:    payload.Usage.CompletionTokens,
		LatencyMS:    latencyMS,
		FinishReason: finishReason,
		Confidence:   extractConfidence(content),
	}
}

func (r OpenAICompatibleRuntime) modelForClass(class string) string {
	switch class {
	case ModelClassMicroSolver:
		return strings.TrimSpace(r.Models.MicroSolver)
	default:
		return ""
	}
}

func promptForRequest(req ModelRequest) []openAIChatMessage {
	inputJSON, _ := json.Marshal(req.Input)
	return []openAIChatMessage{
		{
			Role:    "system",
			Content: baseSystemPrompt(req.Task),
		},
		{
			Role:    "user",
			Content: fmt.Sprintf("task=%s\nschema=%s\n%s\ninput=%s", req.Task, req.SchemaName, taskOutputContract(req.Task), string(inputJSON)),
		},
	}
}

func baseSystemPrompt(task string) string {
	return "You are a typed patch solver inside Needle-X. Return exactly one compact JSON object only. " +
		"Never include markdown, code fences, commentary, or extra keys. " +
		contractForTask(task)
}

func taskOutputContract(task string) string {
	switch task {
	case TaskResolveAmbiguity:
		return `return JSON only with keys: selected_chunk_ids:string[], rejected_chunk_ids:string[], decision_reason:string, confidence:number. confidence must be between 0 and 1.`
	case TaskQueryRewrite:
		return `return JSON only with keys: search_queries:string[], canonical_entity:string, locality_hints:string[], category_hints:string[], confidence:number. search_queries must contain 1 to 5 concise search strings. Prefer semantically faithful multilingual retrieval variants over surface-form copying. canonical_entity may be empty when no stable entity anchor is needed. confidence must be between 0 and 1.`
	case TaskEndpointExtract:
		return `return JSON only with keys: selected_url:string, evidence_page_url:string, kind:string, confidence:number. selected_url must be one literal URL from the provided candidate_pages literal_urls list or one provided page_url. kind must be one of native_api_endpoint, compat_api_endpoint, docs_page, unrelated. confidence must be between 0 and 1.`
	default:
		return "return JSON only."
	}
}

func contractForTask(task string) string {
	switch task {
	case TaskResolveAmbiguity:
		return "For resolve_ambiguity, emit only the four required keys and use chunk ids exactly as provided."
	case TaskQueryRewrite:
		return "For query_rewrite, rewrite the user goal into up to five concise search queries that preserve meaning, intent, and entity identity without requiring exact surface-form repetition. Prefer semantically complementary variants that improve multilingual retrieval, avoid speculation, and do not invent facts not implied by the input."
	case TaskEndpointExtract:
		return "For endpoint_extract, choose the most likely native provider endpoint or API base URL from the provided literal URLs. Prefer first-party native endpoints over proxies, compatibility layers, billing pages, policy pages, and generic docs. Never invent a URL and never output a URL that is not present in the input."
	default:
		return ""
	}
}

func extractConfidence(raw string) float64 {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return 0
	}
	value, ok := payload["confidence"]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return typed
	default:
		return 0
	}
}
