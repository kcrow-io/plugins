package kindenv

import (
	"context"
	"fmt"
	"time"
)

// RolloutStatus waits for a resource rollout to finish.
func (c *Cluster) RolloutStatus(ctx context.Context, resource string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"rollout", "status", resource, "-n", "kube-system", "--timeout", timeout.String()}
	if _, stderr, err := c.Kubectl(ctx, args...); err != nil {
		return fmt.Errorf("kubectl rollout status failed: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// WaitForJobCompletion waits for the named Job to complete.
func (c *Cluster) WaitForJobCompletion(ctx context.Context, namespace, job string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := []string{"wait", "--for=condition=complete", fmt.Sprintf("job/%s", job), "-n", namespace, "--timeout", timeout.String()}
	if _, stderr, err := c.Kubectl(ctx, args...); err != nil {
		return fmt.Errorf("wait for job completion failed: %w (stderr: %s)", err, stderr)
	}
	return nil
}
