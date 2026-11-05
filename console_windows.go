//go:build windows

package main

import "syscall"

// initConsole memaksa halaman kode konsol ke UTF-8 supaya karakter seperti
// pohon (▸ ▾), emoji, dan panah tampil benar. Tanpa ini konsol Windows memakai
// CP 437/1252 dan byte UTF-8 tampil sebagai karakter acak.
func initConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	_, _, _ = setConsoleOutputCP.Call(uintptr(65001))
}
