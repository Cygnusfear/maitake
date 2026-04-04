package test

import (
	"os"
	"testing"
)

// TestFallback_PrGoDeleted confirms cmd/mai/pr.go no longer exists.
func TestFallback_PrGoDeleted(t *testing.T) {
	path := projectRoot() + "/cmd/mai/pr.go"
	if _, err := os.Stat(path); err == nil {
		t.Error("cmd/mai/pr.go should be deleted — PR logic lives in cmd/mai-pr/")
	}
}

// TestFallback_LatticeGoDeleted confirms cmd/mai/lattice.go no longer exists.
func TestFallback_LatticeGoDeleted(t *testing.T) {
	path := projectRoot() + "/cmd/mai/lattice.go"
	if _, err := os.Stat(path); err == nil {
		t.Error("cmd/mai/lattice.go should be deleted — docs logic lives in cmd/mai-docs/")
	}
}

// TestFallback_DaemonGoDeleted confirms cmd/mai/daemon.go no longer exists.
func TestFallback_DaemonGoDeleted(t *testing.T) {
	path := projectRoot() + "/cmd/mai/daemon.go"
	if _, err := os.Stat(path); err == nil {
		t.Error("cmd/mai/daemon.go should be deleted — daemon logic lives in cmd/mai-docs/")
	}
}
