package main

import (
	"os/exec"
	"syscall"
)

// hideWindow menyembunyikan jendela konsol proses anak di Windows, supaya
// pemanggilan PowerShell tidak menampilkan kedipan jendela hitam.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
