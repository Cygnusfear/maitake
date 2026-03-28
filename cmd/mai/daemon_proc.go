//go:build !windows

package main

import "syscall"

// daemonSysProcAttr returns platform-specific attributes to detach the daemon.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true, // new session — survives parent exit
	}
}
