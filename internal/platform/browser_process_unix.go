//go:build !windows

package platform

import (
	"os/exec"
	"syscall"
)

func configureBrowserProcess(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

func killBrowserProcess(cmd *exec.Cmd) error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
