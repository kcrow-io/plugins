package helpers

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCommandRunSuccess(t *testing.T) {
	cmd := Command{
		Name: "sh",
		Args: []string{"-c", "echo hello"},
	}
	stdout, stderr, err := cmd.Run(context.Background())
	if err != nil {
		t.Fatalf("expected success, got err: %v (stderr=%s)", err, stderr)
	}
	if strings.TrimSpace(stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestCommandRunTimeout(t *testing.T) {
	cmd := Command{
		Name:    "sh",
		Args:    []string{"-c", "sleep 2"},
		Timeout: 250 * time.Millisecond,
	}
	_, _, err := cmd.Run(context.Background())
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("expected timeout error message, got %v", err)
	}
}
