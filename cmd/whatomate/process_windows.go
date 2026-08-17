//go:build windows

package main

import (
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {
	// Not supported on Windows in the same way, do nothing
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd != nil && cmd.Process != nil {
		return cmd.Process.Kill()
	}
	return nil
}
