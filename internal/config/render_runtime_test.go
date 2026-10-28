package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderRuntimeLimitsLoadFromOwnerAndRejectInvalidValues(t *testing.T) {
	for _, tc := range []struct {
		json  string
		valid bool
	}{
		{`{"render":{"browser_idle_timeout_ms":1700,"browser_max_uses":3,"max_concurrency":2}}`, true},
		{`{"render":{"browser_idle_timeout_ms":0}}`, false},
		{`{"render":{"browser_max_uses":-1}}`, false},
		{`{"render":{"max_concurrency":0}}`, false},
	} {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(tc.json), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(path)
		if tc.valid {
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Render.BrowserIdleTimeoutMS != 1700 || cfg.Render.BrowserMaxUses != 3 || cfg.Render.MaxConcurrency != 2 {
				t.Fatal("owner override was lost")
			}
		} else if err == nil {
			t.Fatalf("accepted invalid config %s", tc.json)
		}
	}
}
