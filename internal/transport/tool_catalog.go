package transport

import "fmt"

type openAIToolCatalog struct {
	Provider string                 `json:"provider"`
	Strict   bool                   `json:"strict,omitempty"`
	Tools    []openAIToolDefinition `json:"tools"`
}

type openAIToolDefinition struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict,omitempty"`
}

type anthropicToolCatalog struct {
	Provider string                    `json:"provider"`
	Tools    []anthropicToolDefinition `json:"tools"`
}

type anthropicToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

func buildOpenAIToolCatalog(strict bool) openAIToolCatalog {
	tools := mcpTools()
	out := make([]openAIToolDefinition, 0, len(tools))
	for _, tool := range tools {
		fn := openAIFunction{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
			Strict:      strict,
		}
		out = append(out, openAIToolDefinition{
			Type:     "function",
			Function: fn,
		})
	}
	return openAIToolCatalog{
		Provider: "openai",
		Strict:   strict,
		Tools:    out,
	}
}

func buildAnthropicToolCatalog() anthropicToolCatalog {
	tools := mcpTools()
	out := make([]anthropicToolDefinition, 0, len(tools))
	for _, tool := range tools {
		out = append(out, anthropicToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return anthropicToolCatalog{
		Provider: "anthropic",
		Tools:    out,
	}
}

func toolCatalog(provider string, strict bool) (any, error) {
	switch provider {
	case "openai":
		return buildOpenAIToolCatalog(strict), nil
	case "anthropic":
		return buildAnthropicToolCatalog(), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}
