package rendering

import (
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"path"
	"sort"
	"strings"
)

const (
	maxRenderNetworkBlocks     = 4096
	maxRenderNetworkTextBytes  = 16_000_000
	maxRenderNetworkValueChars = 1200
)

func EvidenceText(resources []NetworkResource) string {
	blocks := []string{}
	totalBytes := 0
	appendBlock := func(block string) bool {
		block = normalizeRenderNetworkText(block)
		if block == "" {
			return true
		}
		blocks = append(blocks, block)
		totalBytes += len([]byte(block))
		return len(blocks) < maxRenderNetworkBlocks && totalBytes < maxRenderNetworkTextBytes
	}
	for _, resource := range resources {
		if strings.TrimSpace(resource.Body) == "" {
			continue
		}
		header := renderNetworkResourceHeader(resource)
		if header != "" && !appendBlock(header) {
			return strings.Join(blocks, "\n\n")
		}
		for _, block := range renderNetworkResourceBlocks(resource) {
			if !appendBlock(block) {
				return strings.Join(blocks, "\n\n")
			}
		}
	}
	return strings.Join(blocks, "\n\n")
}

func renderNetworkResourceHeader(resource NetworkResource) string {
	label := strings.TrimSpace(resource.Type)
	if label == "" {
		label = strings.TrimSpace(resource.Source)
	}
	parsed, err := url.Parse(strings.TrimSpace(resource.URL))
	if err == nil && strings.TrimSpace(parsed.Path) != "" {
		return fmt.Sprintf("Render network resource %s %s", label, path.Base(parsed.Path))
	}
	if strings.TrimSpace(resource.URL) != "" {
		return fmt.Sprintf("Render network resource %s %s", label, resource.URL)
	}
	if label != "" {
		return "Render network resource " + label
	}
	return ""
}

func renderNetworkResourceBlocks(resource NetworkResource) []string {
	body := strings.TrimSpace(resource.Body)
	if body == "" {
		return nil
	}
	if strings.Contains(strings.ToLower(resource.ContentType), "event-stream") || strings.EqualFold(resource.Type, "EventSource") || strings.EqualFold(resource.Source, "event_source") {
		return renderEventStreamBlocks(body)
	}
	if blocks := renderJSONBlocks(body); len(blocks) > 0 {
		return blocks
	}
	return renderPlainNetworkTextBlocks(body)
}

func renderEventStreamBlocks(body string) []string {
	blocks := []string{}
	for _, event := range eventStreamDataBlocks(body) {
		event = strings.TrimSpace(event)
		if event == "" || event == "end" {
			continue
		}
		if jsonBlocks := renderJSONBlocks(event); len(jsonBlocks) > 0 {
			blocks = append(blocks, jsonBlocks...)
		} else {
			blocks = append(blocks, normalizeRenderNetworkText(event))
		}
		if len(blocks) >= maxRenderNetworkBlocks {
			break
		}
	}
	return blocks
}

func eventStreamDataBlocks(body string) []string {
	events := []string{}
	current := []string{}
	flush := func() {
		if len(current) == 0 {
			return
		}
		events = append(events, strings.Join(current, "\n"))
		current = nil
	}
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimRight(line, "\r")
		if strings.TrimSpace(trimmed) == "" {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			current = append(current, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}
	}
	flush()
	return events
}

func renderJSONBlocks(raw string) []string {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	return renderJSONValueBlocks(value)
}

func renderJSONValueBlocks(value any) []string {
	switch typed := value.(type) {
	case []any:
		blocks := []string{}
		for _, item := range typed {
			if block := renderJSONObjectBlock(item); block != "" {
				blocks = append(blocks, block)
			} else {
				blocks = append(blocks, renderJSONValueBlocks(item)...)
			}
			if len(blocks) >= maxRenderNetworkBlocks {
				break
			}
		}
		return blocks
	case map[string]any:
		if block := renderJSONObjectBlock(typed); block != "" {
			return []string{block}
		}
	}
	if block := renderJSONObjectBlock(value); block != "" {
		return []string{block}
	}
	return nil
}

func renderJSONObjectBlock(value any) string {
	values := []string{}
	collectRenderJSONScalars(value, &values, 48)
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, " | ")
}

func collectRenderJSONScalars(value any, out *[]string, maxValues int) {
	if len(*out) >= maxValues {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := typed[key]
			collectRenderJSONNamedScalar(key, child, out, maxValues)
			if len(*out) >= maxValues {
				return
			}
		}
	case []any:
		for _, child := range typed {
			collectRenderJSONScalars(child, out, maxValues)
			if len(*out) >= maxValues {
				return
			}
		}
	case string:
		addRenderJSONScalar("", typed, out)
	case json.Number:
		addRenderJSONScalar("", typed.String(), out)
	case float64, bool:
		addRenderJSONScalar("", fmt.Sprint(typed), out)
	}
}

func collectRenderJSONNamedScalar(key string, value any, out *[]string, maxValues int) {
	switch typed := value.(type) {
	case string:
		addRenderJSONScalar(key, typed, out)
	case json.Number:
		addRenderJSONScalar(key, typed.String(), out)
	case float64, bool:
		addRenderJSONScalar(key, fmt.Sprint(typed), out)
	case []any, map[string]any:
		collectRenderJSONScalars(typed, out, maxValues)
	}
}

func addRenderJSONScalar(key, value string, out *[]string) {
	value = normalizeRenderNetworkScalarValue(html.UnescapeString(value))
	if !renderNetworkValueUseful(value) {
		return
	}
	if len([]rune(value)) > maxRenderNetworkValueChars {
		value = string([]rune(value)[:maxRenderNetworkValueChars])
	}
	key = normalizeRenderNetworkText(key)
	if key != "" && len(key) <= 48 && renderNetworkKeyUseful(key) {
		value = key + ": " + value
	}
	*out = append(*out, value)
}

func renderPlainNetworkTextBlocks(body string) []string {
	lines := strings.Split(body, "\n")
	blocks := []string{}
	for _, line := range lines {
		line = normalizeRenderNetworkScalarValue(line)
		if !renderNetworkValueUseful(line) {
			continue
		}
		blocks = append(blocks, line)
		if len(blocks) >= maxRenderNetworkBlocks {
			break
		}
	}
	return blocks
}

func normalizeRenderNetworkText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
