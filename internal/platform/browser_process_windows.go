package platform

import "os/exec"

func prepareBrowserProcess(*exec.Cmd) (func() error, error) {
	return func() error { return nil }, nil
}

func killBrowserProcess(cmd *exec.Cmd) error { return cmd.Process.Kill() }
