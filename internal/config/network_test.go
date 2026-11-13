package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/josepavese/needlex/internal/platform"
)

func TestNetworkPolicyDefaultsToAny(t *testing.T) {
	cfg := Defaults()
	if cfg.Network.Policy != string(platform.NetworkPolicyAny) {
		t.Fatalf("default network policy = %q", cfg.Network.Policy)
	}
	if cfg.EgressPolicy() != platform.NetworkPolicyAny {
		t.Fatal("default egress policy is not any")
	}
}

func TestNetworkPolicyValidation(t *testing.T) {
	cfg := Defaults()
	cfg.Network.Policy = "public"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("public policy rejected: %v", err)
	}
	cfg.Network.Policy = "internet"
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "network.policy") {
		t.Fatalf("invalid policy error = %v", err)
	}
}

func TestNetworkPolicyEnvironmentOverride(t *testing.T) {
	t.Setenv("NEEDLEX_NETWORK_POLICY", "public")
	cfg, err := DefaultsWithEnv()
	if err != nil {
		t.Fatalf("defaults with env: %v", err)
	}
	if cfg.Network.Policy != string(platform.NetworkPolicyPublic) || cfg.EgressPolicy() != platform.NetworkPolicyPublic {
		t.Fatalf("env policy = %q", cfg.Network.Policy)
	}
}

func TestNetworkPolicyLoadsFromConfigJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "needlex.json")
	if err := os.WriteFile(path, []byte(`{"network":{"policy":"public"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.EgressPolicy() != platform.NetworkPolicyPublic {
		t.Fatalf("loaded policy = %q", cfg.Network.Policy)
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"network":{"policy":"public"}`) {
		t.Fatalf("serialized config does not expose network policy: %s", data)
	}
}
