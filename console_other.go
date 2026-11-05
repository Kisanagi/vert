//go:build !windows

package main

// initConsole tidak melakukan apa pun di luar Windows.
func initConsole() {}
