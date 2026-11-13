package rendering

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
)

func TestRenderPublicPolicyDeniesInitialURLBeforeLaunch(t *testing.T) {
	runtime, err := NewRuntime(config.DefaultRenderConfig(), platform.NetworkPolicyPublic)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close() //nolint:errcheck
	for _, tc := range []struct {
		url  string
		code string
	}{
		{"http://127.0.0.1:9471/", platform.EgressCodeAddress},
		{"http://[::1]/", platform.EgressCodeAddress},
		{"http://[::ffff:10.0.0.1]/", platform.EgressCodeAddress},
		{"http://user:secret@example.com/", platform.EgressCodeUserinfo},
		{"file:///etc/passwd", platform.EgressCodeScheme},
	} {
		_, err := runtime.Render(context.Background(), Request{URL: tc.url, Timeout: time.Second})
		var denied *platform.EgressDeniedError
		if !errors.As(err, &denied) || denied.Code != tc.code {
			t.Errorf("%s: render error = %v, want code %s", tc.url, err, tc.code)
		}
	}
}

func TestOneShotRendererCarriesNetworkPolicy(t *testing.T) {
	renderer := New(config.DefaultRenderConfig(), platform.NetworkPolicyPublic)
	_, err := renderer.Render(context.Background(), Request{URL: "http://127.0.0.1:9471/", Timeout: time.Second})
	if !platform.IsEgressDenied(err) {
		t.Fatalf("one-shot render error = %v, want egress denial", err)
	}
}
