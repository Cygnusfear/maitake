//go:build windows

package main

import (
	"fmt"
	"os"
)

// acquireDaemonLock is a stub on Windows — always succeeds.
// TODO: implement LockFileEx for Windows exclusive lock.
func acquireDaemonLock() (*os.File, error) {
	return nil, fmt.Errorf("daemon lock not supported on Windows")
}
