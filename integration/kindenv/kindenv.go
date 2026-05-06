package kindenv

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kcrow-io/plugins/integration/helpers"
)

// Cluster wraps a KinD cluster and related helpers such as kubeconfig path.
type Cluster struct {
	Name       string
	Kubeconfig string
}

// CreateCluster provisions a KinD cluster with the provided configuration.
func CreateCluster(ctx context.Context, name, configPath string) (*Cluster, error) {
	if name == "" {
		name = "nri-limit-e2e"
	}
	if err := ensureBinary("kind"); err != nil {
		return nil, err
	}

	config := `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  ipFamily: ipv4`

	args := []string{"create", "cluster", "-v7", "--wait", "5m", "--retain", "--name", name, "--config", "-"}

	cmd := helpers.Command{
		Name:    "kind",
		Args:    args,
		Timeout: 5 * time.Minute,
		Stdin:   []byte(config),
	}
	if _, stderr, err := cmd.Run(ctx); err != nil {
		return nil, fmt.Errorf("create cluster: %w (stderr: %s)", err, stderr)
	}

	kubeconfig, err := kubeconfigFor(ctx, name)
	if err != nil {
		return nil, err
	}

	return &Cluster{
		Name:       name,
		Kubeconfig: kubeconfig,
	}, nil
}

// Destroy tears down the cluster.
func (c *Cluster) Destroy(ctx context.Context) error {
	if c == nil || c.Name == "" {
		return nil
	}
	cmd := helpers.Command{
		Name:    "kind",
		Args:    []string{"delete", "cluster", "--name", c.Name},
		Timeout: 2 * time.Minute,
	}
	_, stderr, err := cmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("delete cluster: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// Kubectl invokes kubectl with the cluster kubeconfig.
func (c *Cluster) Kubectl(ctx context.Context, args ...string) (string, string, error) {
	if err := ensureBinary("kubectl"); err != nil {
		return "", "", err
	}
	cmd := helpers.Command{
		Name:    "kubectl",
		Args:    append([]string{"--kubeconfig", c.Kubeconfig}, args...),
		Timeout: 2 * time.Minute,
	}
	return cmd.Run(ctx)
}

func kubeconfigFor(ctx context.Context, name string) (string, error) {
	cmd := helpers.Command{
		Name:    "kind",
		Args:    []string{"get", "kubeconfig", "--name", name},
		Timeout: 30 * time.Second,
	}
	stdout, stderr, err := cmd.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("get kubeconfig: %w (stderr: %s)", err, stderr)
	}

	dir, err := os.MkdirTemp("", "kind-kubeconfig-*")
	if err != nil {
		return "", fmt.Errorf("create kubeconfig temp dir: %w", err)
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(stdout), 0o600); err != nil {
		return "", fmt.Errorf("write kubeconfig: %w", err)
	}
	return path, nil
}

func ensureBinary(name string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s binary is required in PATH: %w", name, err)
	}
	return nil
}

// ApplyManifest applies the provided manifest string against the cluster.
func (c *Cluster) ApplyManifest(ctx context.Context, manifest string) error {
	if strings.TrimSpace(manifest) == "" {
		return errors.New("manifest cannot be empty")
	}
	cmd := helpers.Command{
		Name:    "kubectl",
		Args:    []string{"--kubeconfig", c.Kubeconfig, "apply", "-f", "-"},
		Stdin:   []byte(manifest),
		Timeout: 2 * time.Minute,
	}
	stdout, stderr, err := cmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("kubectl apply failed: %w (stderr: %s, stdout: %s)", err, stderr, stdout)
	}
	return nil
}
