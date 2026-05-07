package transport

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/config"
)

func TestConfigInitShowAndSetUsePALSSOT(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "configs", "needlex.json")
	t.Setenv("NEEDLEX_CONFIG", path)
	runner := NewRunner()
	runner.storeRoot = root

	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"config", "init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("config init exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"config", "set", "semantic.provider_model", "nomic-embed-text:latest"}, &stdout, &stderr); code != 0 {
		t.Fatalf("config set exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	loaded, err := config.Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if loaded.Semantic.ProviderModel != "nomic-embed-text:latest" {
		t.Fatalf("expected updated provider model, got %q", loaded.Semantic.ProviderModel)
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"config", "show"}, &stdout, &stderr); code != 0 {
		t.Fatalf("config show exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "nomic-embed-text:latest") {
		t.Fatalf("expected config show to include updated model, got %q", stdout.String())
	}
}

func TestConfigSetRejectsDisablingSemantic(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "configs", "needlex.json")
	t.Setenv("NEEDLEX_CONFIG", path)
	runner := NewRunner()
	runner.storeRoot = root

	var stdout, stderr bytes.Buffer
	if code := runner.Run([]string{"config", "init"}, &stdout, &stderr); code != 0 {
		t.Fatalf("config init exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := runner.Run([]string{"config", "set", "semantic." + "enabled", "false"}, &stdout, &stderr); code != 2 {
		t.Fatalf("expected config set failure exit=2, got %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
