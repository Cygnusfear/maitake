//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// acquireDaemonLock tries to take an exclusive flock on ~/.maitake/daemon.lock.
// Returns the open file (caller must keep it open for the lock to hold) or an
// error if another daemon already owns it.
func acquireDaemonLock() (*os.File, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(home, ".maitake", "daemon.lock")
	os.MkdirAll(filepath.Dir(lockPath), 0755)

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	// LOCK_EX | LOCK_NB — exclusive, non-blocking.
	// Fails immediately if another process holds the lock.
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		return nil, err
	}

	return f, nil
}
