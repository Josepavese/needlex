package transport

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/rendering"
)

type recordingRenderRuntime struct{ closes int }

func (*recordingRenderRuntime) Render(context.Context, rendering.Request) (rendering.Page, error) {
	return rendering.Page{}, nil
}
func (r *recordingRenderRuntime) Close() error { r.closes++; return nil }

func TestMCPRenderRuntimeSharesReplacesAndCloses(t *testing.T) {
	var created []*recordingRenderRuntime
	runner := Runner{newRenderRuntime: func(config.RenderConfig) (rendering.Runtime, error) {
		r := &recordingRenderRuntime{}
		created = append(created, r)
		return r, nil
	}}
	owner := runner.newMCPRenderRuntime()
	cfg := config.DefaultRenderConfig()
	first, err := owner.get(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.get(cfg)
	if err != nil || first != second {
		t.Fatal("same config did not reuse runtime")
	}
	cfg.MaxConcurrency = 3
	if _, err := owner.get(cfg); err != nil {
		t.Fatal(err)
	}
	if created[0].closes != 1 {
		t.Fatal("config replacement did not close previous runtime")
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if created[1].closes != 1 {
		t.Fatal("shutdown did not close runtime")
	}
	if _, err := owner.get(cfg); err == nil {
		t.Fatal("runtime reopened after shutdown")
	}
}

func TestMCPRunOwnsRendererOnReadErrorAndEOF(t *testing.T) {
	for _, input := range []string{"", "Content-Length: invalid\r\n\r\n"} {
		t.Run(input, func(t *testing.T) {
			r := NewRunner()
			r.storeRoot = t.TempDir()
			r.stdin = strings.NewReader(input)
			// No rendering command means no native runtime is created at all.
			r.newRenderRuntime = func(config.RenderConfig) (rendering.Runtime, error) {
				t.Fatal("eager browser runtime")
				return nil, nil
			}
			r.runMCP(nil, io.Discard, io.Discard)
		})
	}
}
