package intel

import (
	"context"
	"net/http"
	"strings"

	"github.com/josepavese/needlex/internal/config"
)

type ModelRuntime interface {
	Run(ctx context.Context, req ModelRequest) (ModelResponse, error)
}

type NoopRuntime struct {
	Name string
}

func DefaultRuntime() NoopRuntime {
	return NoopRuntime{Name: "noop-runtime"}
}

func (r NoopRuntime) Run(_ context.Context, req ModelRequest) (ModelResponse, error) {
	if err := req.Validate(); err != nil {
		return ModelResponse{}, err
	}
	resp := ModelResponse{
		Model:        firstNonEmptyModel(r.Name),
		Task:         req.Task,
		OutputJSON:   `{"status":"noop"}`,
		TokensIn:     0,
		TokensOut:    0,
		LatencyMS:    0,
		FinishReason: ModelFinishReasonNoop,
		Confidence:   0,
	}
	if err := resp.Validate(); err != nil {
		return ModelResponse{}, err
	}
	return resp, nil
}

func firstNonEmptyModel(name string) string {
	if name != "" {
		return name
	}
	return "noop-runtime"
}

func NewRuntime(cfg config.Config, client *http.Client) ModelRuntime {
	switch strings.TrimSpace(cfg.Models.Backend) {
	case "openai-compatible":
		return OpenAICompatibleRuntime{
			BaseURL: cfg.Models.BaseURL,
			APIKey:  cfg.Models.APIKey,
			Client:  client,
			Models: RuntimeModels{
				MicroSolver: cfg.Models.Router,
			},
		}
	case "ollama":
		return OllamaRuntime{
			BaseURL: cfg.Models.BaseURL,
			Client:  client,
			Models: RuntimeModels{
				MicroSolver: cfg.Models.Router,
			},
		}
	default:
		return DefaultRuntime()
	}
}
