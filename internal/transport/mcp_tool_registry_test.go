package transport

import "testing"

func TestMCPToolRegistryIsSingleSourceForDefinitionsAndHandlers(t *testing.T) {
	definitions := mcpTools()
	handlers := mcpToolHandlersByName
	if len(definitions) == 0 {
		t.Fatal("expected MCP tool definitions")
	}
	if len(definitions) != len(handlers) {
		t.Fatalf("definitions=%d handlers=%d", len(definitions), len(handlers))
	}
	seen := map[string]struct{}{}
	for _, def := range definitions {
		if def.Name == "" {
			t.Fatalf("tool with empty name: %+v", def)
		}
		if _, ok := seen[def.Name]; ok {
			t.Fatalf("duplicate tool definition %q", def.Name)
		}
		seen[def.Name] = struct{}{}
		if handlers[def.Name] == nil {
			t.Fatalf("missing handler for %q", def.Name)
		}
	}
}
