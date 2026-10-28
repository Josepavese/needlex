//go:build !windows

package platform

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrowserProcessIsolatesShortSocketTempDirectory(t *testing.T) {
	parent := filepath.Join(t.TempDir(), strings.Repeat("long-parent-", 10))
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", parent)
	cmd := exec.Command("sh", "-c", `printf '%s' "$TMPDIR"`)
	cleanup, err := prepareBrowserProcess(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	dir := string(output)
	if dir == parent || os.Getenv("TMPDIR") != parent {
		t.Fatal("browser temporary environment was not isolated")
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("private socket directory: info=%v err=%v", info, err)
	}
	socketDir, err := os.MkdirTemp(dir, "org.chromium.Chromium.")
	if err != nil {
		t.Fatal(err)
	}
	socket, err := net.Listen("unix", filepath.Join(socketDir, "SingletonSocket"))
	if err != nil {
		t.Fatalf("Chromium-style Unix socket under long parent TMPDIR: %v", err)
	}
	_ = socket.Close()
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("socket directory survived release: %v", err)
	}
}
