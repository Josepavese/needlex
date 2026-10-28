//go:build !windows

package platform

import (
	"os"
	"os/exec"
	"syscall"
)

const browserSocketTempRoot = "/tmp"

func prepareBrowserProcess(cmd *exec.Cmd) (func() error, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Chromium creates Unix sockets beneath TMPDIR. Keep that private path short
	// even when the caller's temporary/profile directory has a long name.
	dir, err := os.MkdirTemp(browserSocketTempRoot, "needlex-ipc-*")
	if err != nil {
		return nil, err
	}
	cmd.Env = append(cmd.Environ(), "TMPDIR="+dir)
	return func() error { return os.RemoveAll(dir) }, nil
}

func killBrowserProcess(cmd *exec.Cmd) error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
