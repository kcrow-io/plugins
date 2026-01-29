//go:build e2e

package integration

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kcrow-io/plugins/integration/helpers"
	"github.com/kcrow-io/plugins/integration/kindenv"
)

const (
	diskJobName   = "limit-disk-stress"
	memoryJobName = "limit-memory-stress"
)

func TestLimitPluginE2E(t *testing.T) {
	ctx := context.Background()
	requireCommands(t, "docker", "kind", "kubectl")

	manifestDir := manifestsDir(t)

	// Build binary using make build
	binPath, confPath := buildLimitBinary(t, ctx)

	cluster, err := kindenv.CreateCluster(ctx, "limit-e2e", filepath.Join(manifestDir, "kind-config.yaml"))
	if err != nil {
		t.Fatalf("failed to create KinD cluster: %v", err)
	}
	t.Cleanup(func() {
		cluster.Destroy(context.Background())
	})

	// Install plugin directly into kind nodes
	if err := installPluginIntoNodes(ctx, cluster.Name, binPath, confPath); err != nil {
		t.Fatalf("failed to install plugin into nodes: %v", err)
	}

	applyJobAndWait(t, ctx, cluster, filepath.Join(manifestDir, "stress-disk-job.yaml"),
		map[string]string{"__DISK_JOB_NAME__": diskJobName}, "default", diskJobName)

	if err := waitForPluginLog(ctx, cluster.Name, "Applied io limit", 2*time.Minute); err != nil {
		t.Fatalf("io throttling log not observed: %v", err)
	}

	applyJobAndWait(t, ctx, cluster, filepath.Join(manifestDir, "stress-memory-job.yaml"),
		map[string]string{"__MEMORY_JOB_NAME__": memoryJobName}, "default", memoryJobName)

	if err := waitForPluginLog(ctx, cluster.Name, "memory exceeds", 2*time.Minute); err != nil {
		t.Fatalf("memory cache clearing log not observed: %v", err)
	}
}

func requireCommands(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s not found in PATH: %v", name, err)
		}
	}
}

func buildLimitBinary(t *testing.T, ctx context.Context) (string, string) {
	t.Helper()
	root := repoRoot(t)

	// Run make build
	buildCmd := exec.CommandContext(ctx, "make", "build")
	buildCmd.Dir = root
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to build limit binary with make: %v", err)
	}

	// The binary is built as bin/linux/amd64/07-limit
	binPath := filepath.Join(root, "bin", "linux", "amd64", "07-limit")
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("binary not found at %s: %v", binPath, err)
	}

	// The config file is at bin/linux/amd64/limit.conf
	confPath := filepath.Join(root, "bin", "linux", "amd64", "limit.conf")
	if _, err := os.Stat(confPath); err != nil {
		t.Fatalf("config file not found at %s: %v", confPath, err)
	}

	return binPath, confPath
}

func installPluginIntoNodes(ctx context.Context, clusterName, binPath, confPath string) error {
	// Get list of nodes in the cluster
	cmd := helpers.Command{
		Name:    "docker",
		Args:    []string{"ps", "-q", "--filter", fmt.Sprintf("name=%s-", clusterName)},
		Timeout: 30 * time.Second,
	}
	stdout, stderr, err := cmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w (stderr: %s)", err, stderr)
	}

	nodeIDs := strings.Fields(strings.TrimSpace(stdout))
	if len(nodeIDs) == 0 {
		return fmt.Errorf("no nodes found for cluster %s", clusterName)
	}

	for _, nodeID := range nodeIDs {
		// Create directories in the node
		createDirsCmd := helpers.Command{
			Name:    "docker",
			Args:    []string{"exec", nodeID, "mkdir", "-p", "/opt/nri/plugins", "/etc/nri/conf.d"},
			Timeout: 30 * time.Second,
		}
		if _, stderr, err := createDirsCmd.Run(ctx); err != nil {
			return fmt.Errorf("failed to create directories in node %s: %w (stderr: %s)", nodeID, err, stderr)
		}

		// Copy binary to /opt/nri/plugins/
		copyBinCmd := helpers.Command{
			Name:    "docker",
			Args:    []string{"cp", binPath, fmt.Sprintf("%s:/opt/nri/plugins/06-limit", nodeID)},
			Timeout: 30 * time.Second,
		}
		if _, stderr, err := copyBinCmd.Run(ctx); err != nil {
			return fmt.Errorf("failed to copy binary to node %s: %w (stderr: %s)", nodeID, err, stderr)
		}

		// Make binary executable
		chmodCmd := helpers.Command{
			Name:    "docker",
			Args:    []string{"exec", nodeID, "chmod", "+x", "/opt/nri/plugins/06-limit"},
			Timeout: 30 * time.Second,
		}
		if _, stderr, err := chmodCmd.Run(ctx); err != nil {
			return fmt.Errorf("failed to chmod binary in node %s: %w (stderr: %s)", nodeID, err, stderr)
		}

		// Copy config to /etc/nri/conf.d/
		copyConfCmd := helpers.Command{
			Name:    "docker",
			Args:    []string{"cp", confPath, fmt.Sprintf("%s:/etc/nri/conf.d/limit.conf", nodeID)},
			Timeout: 30 * time.Second,
		}
		if _, stderr, err := copyConfCmd.Run(ctx); err != nil {
			return fmt.Errorf("failed to copy config to node %s: %w (stderr: %s)", nodeID, err, stderr)
		}

		// Restart containerd
		restartCmd := helpers.Command{
			Name:    "docker",
			Args:    []string{"exec", nodeID, "service", "containerd", "restart"},
			Timeout: 1 * time.Minute,
		}
		if _, stderr, err := restartCmd.Run(ctx); err != nil {
			return fmt.Errorf("failed to restart containerd in node %s: %w (stderr: %s)", nodeID, err, stderr)
		}

		// Wait a bit for containerd to fully restart
		time.Sleep(5 * time.Second)
	}

	return nil
}

func readManifest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest %s: %v", path, err)
	}
	return string(data)
}

func applyJobAndWait(t *testing.T, ctx context.Context, cluster *kindenv.Cluster, manifestPath string, replacements map[string]string, namespace, jobName string) {
	t.Helper()
	if _, _, err := cluster.Kubectl(ctx, "delete", "job", jobName, "-n", namespace, "--ignore-not-found"); err != nil {
		t.Logf("job cleanup failed (non-fatal): %v", err)
	}
	manifest := readManifest(t, manifestPath)
	for k, v := range replacements {
		manifest = strings.ReplaceAll(manifest, k, v)
	}
	if err := cluster.ApplyManifest(ctx, manifest); err != nil {
		t.Fatalf("failed to apply manifest %s: %v", manifestPath, err)
	}
	if err := cluster.WaitForJobCompletion(ctx, namespace, jobName, 5*time.Minute); err != nil {
		t.Fatalf("job %s did not complete: %v", jobName, err)
	}
}

func runOrLog(t *testing.T, ctx context.Context, name string, args ...string) {
	t.Helper()
	cmd := helpers.Command{Name: name, Args: args}
	if _, _, err := cmd.Run(ctx); err != nil {
		t.Logf("command %s %v failed: %v", name, args, err)
	}
}

func waitForLogSubstring(ctx context.Context, cluster *kindenv.Cluster, substr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		stdout, _, err := cluster.Kubectl(ctx, "-n", "nri-system", "logs", "daemonset/nri-limit")
		if err == nil && strings.Contains(stdout, substr) {
			return nil
		}
		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("failed to fetch logs before deadline: %w", err)
			}
			return fmt.Errorf("log substring %q not found before deadline", substr)
		}
		time.Sleep(5 * time.Second)
	}
}

func waitForPluginLog(ctx context.Context, clusterName, substr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	// Get the first node ID
	cmd := helpers.Command{
		Name:    "docker",
		Args:    []string{"ps", "-q", "--filter", fmt.Sprintf("name=%s-", clusterName)},
		Timeout: 30 * time.Second,
	}
	stdout, _, err := cmd.Run(ctx)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	nodeIDs := strings.Fields(strings.TrimSpace(stdout))
	if len(nodeIDs) == 0 {
		return fmt.Errorf("no nodes found for cluster %s", clusterName)
	}
	nodeID := nodeIDs[0]

	for {
		// Read the log file from the node
		readLogCmd := helpers.Command{
			Name:    "docker",
			Args:    []string{"exec", nodeID, "cat", "/var/log/nri-limit.log"},
			Timeout: 30 * time.Second,
		}
		logContent, _, err := readLogCmd.Run(ctx)
		if err == nil && strings.Contains(logContent, substr) {
			return nil
		}

		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("failed to read log file before deadline: %w", err)
			}
			return fmt.Errorf("log substring %q not found before deadline", substr)
		}
		time.Sleep(5 * time.Second)
	}
}

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

func manifestsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "integration", "manifests")
}
