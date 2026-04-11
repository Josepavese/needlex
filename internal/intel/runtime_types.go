package intel

const (
	TaskResolveAmbiguity  = "resolve_ambiguity"
	TaskEndpointExtract   = "endpoint_extract"
	TaskQueryRewrite      = "query_rewrite"
	ModelClassMicroSolver = "micro_solver"
	ModelFinishReasonStop = "stop"
	ModelFinishReasonNoop = "noop"
)

type ModelTraceContext struct {
	RunID        string `json:"run_id,omitempty"`
	TraceID      string `json:"trace_id,omitempty"`
	ChunkID      string `json:"chunk_id,omitempty"`
	Fingerprint  string `json:"fingerprint,omitempty"`
	FailureClass string `json:"failure_class,omitempty"`
	ReasonCode   string `json:"reason_code,omitempty"`
}

type ModelRequest struct {
	Task            string            `json:"task"`
	ModelClass      string            `json:"model_class"`
	MaxInputTokens  int               `json:"max_input_tokens"`
	MaxOutputTokens int               `json:"max_output_tokens"`
	TimeoutMS       int64             `json:"timeout_ms"`
	SchemaName      string            `json:"schema_name"`
	Input           map[string]any    `json:"input"`
	Trace           ModelTraceContext `json:"trace"`
}

type ModelResponse struct {
	Model        string  `json:"model"`
	Task         string  `json:"task"`
	OutputJSON   string  `json:"output_json"`
	TokensIn     int     `json:"tokens_in"`
	TokensOut    int     `json:"tokens_out"`
	LatencyMS    int64   `json:"latency_ms"`
	FinishReason string  `json:"finish_reason"`
	Confidence   float64 `json:"confidence"`
}

func validTask(task string) bool {
	switch task {
	case TaskResolveAmbiguity, TaskQueryRewrite, TaskEndpointExtract:
		return true
	default:
		return false
	}
}

func validModelClass(class string) bool {
	switch class {
	case ModelClassMicroSolver:
		return true
	default:
		return false
	}
}

func validFinishReason(reason string) bool {
	switch reason {
	case ModelFinishReasonStop, ModelFinishReasonNoop:
		return true
	default:
		return false
	}
}
