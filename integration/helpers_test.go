//go:build e2e

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	binaries map[string]string // plugin name -> binary path
)

func TestMain(m *testing.M) {
	// Build all binaries once before any tests
	binaries = buildAllBinaries()
	os.Exit(m.Run())
}

// requireCommands checks if required commands are available in PATH
func requireCommands(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s not found in PATH: %v", name, err)
		}
	}
}

// repoRoot returns the repository root directory
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod starting from %s", dir)
		}
		dir = parent
	}
}

// buildAllBinaries builds all plugin binaries and returns their paths
func buildAllBinaries() map[string]string {
	// Use a temporary testing.T for repoRoot
	t := &testing.T{}
	root := repoRoot(t)

	// Run make build
	buildCmd := exec.CommandContext(context.Background(), "make", "build")
	buildCmd.Dir = root
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		// Log but don't fail - binaries might already exist
		t.Logf("make build failed (binaries may already exist): %v", err)
	}

	// Find binaries
	bins := map[string]string{
		"memory": filepath.Join(root, "bin", "linux", "amd64", "06-memory"),
		"limit":  filepath.Join(root, "bin", "linux", "amd64", "07-limit"),
		"escape": filepath.Join(root, "bin", "linux", "amd64", "08-escape"),
	}

	// Verify binaries exist
	for name, path := range bins {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("binary %s not found at %s: %v", name, path, err)
		}
	}

	return bins
}

// findContainerdPath returns the path to containerd binary
// Priority: 1. CONTAINERD_BIN env var, 2. /usr/local/bin/containerd, 3. PATH lookup
func findContainerdPath(t *testing.T) string {
	t.Helper()

	// Check environment variable first
	if bin := os.Getenv("CONTAINERD_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin
		}
	}

	// Check /usr/local/bin/containerd (install_containerd script location)
	localBin := "/usr/local/bin/containerd"
	if _, err := os.Stat(localBin); err == nil {
		return localBin
	}

	// Fallback to PATH lookup
	path, err := exec.LookPath("containerd")
	if err != nil {
		t.Fatalf("containerd not found: %v", err)
	}

	return path
}
