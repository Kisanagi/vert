//go:build !windows

package main

import "os/exec"

// hideWindow tidak diperlukan di luar Windows.
func hideWindow(cmd *exec.Cmd) {}
