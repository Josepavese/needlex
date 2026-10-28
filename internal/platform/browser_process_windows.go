package platform

import "os/exec"

func configureBrowserProcess(cmd *exec.Cmd)  {}
func killBrowserProcess(cmd *exec.Cmd) error { return cmd.Process.Kill() }
