package rendering

import (
	"encoding/json"
	"testing"
)

func TestRequestAccountingDoesNotChangeCaptureOrIdlePolicy(t *testing.T) {
	c := newNetworkCollector("https://example.com", 1000, 1000, 10, 10)
	activity := c.lastActivity
	for _, raw := range []string{
		`{"requestId":"document","request":{"url":"https://example.com/"}}`,
		`{"requestId":"image","request":{"url":"https://cdn.example.net/image.png"}}`,
		`{"requestId":"redirect","request":{"url":"http://example.com/"}}`,
		`{"requestId":"redirect","request":{"url":"https://example.com/"}}`,
		`{"requestId":"local","request":{"url":"data:text/plain,local"}}`,
		`{"request":{"url":"https://example.com/malformed"}}`,
	} {
		c.handleEvent(cdpMessage{Method: "Network.requestWillBeSent", Params: json.RawMessage(raw)})
	}
	resources, stats := c.snapshot()
	if stats.HTTPRequests != 4 || stats.ObservedResources != 0 || len(resources) != 0 {
		t.Fatalf("request accounting confused with retained evidence: %#v", stats)
	}
	if c.lastActivity != activity {
		t.Fatal("accounting extended the render wait")
	}
}
