package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Command represents an external command invocation with contextual logging.
type Command struct {
	Name    string
	Args    []string
	Env     []string
	Timeout time.Duration
	Stdin   []byte
}

// Run executes the configured command and returns stdout/stderr or error.
func (c Command) Run(ctx context.Context) (string, string, error) {
	if c.Name == "" {
		return "", "", fmt.Errorf("command name is required")
	}

	ctxToUse := ctx
	var cancel context.CancelFunc
	if c.Timeout > 0 {
		ctxToUse, cancel = context.WithTimeout(ctx, c.Timeout)
	} else {
		ctxToUse, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(ctxToUse, c.Name, c.Args...)
	cmd.Env = append(cmd.Env, c.Env...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if len(c.Stdin) > 0 {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}

	err := cmd.Run()
	// When the context was canceled/timeout, surface a more descriptive error.
	if ctxToUse.Err() != nil && err == context.DeadlineExceeded {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("%s timed out after %s", c.String(), c.Timeout)
	}

	return stdoutBuf.String(), stderrBuf.String(), err
}

// String returns a shell-like representation of the command.
func (c Command) String() string {
	parts := []string{c.Name}
	for _, arg := range c.Args {
		if strings.ContainsAny(arg, " \t") {
			parts = append(parts, fmt.Sprintf("%q", arg))
			continue
		}
		parts = append(parts, arg)
	}
	return strings.Join(parts, " ")
}
