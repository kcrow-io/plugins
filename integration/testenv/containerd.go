package testenv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// CgroupVersion represents the cgroup version
type CgroupVersion int

const (
	CgroupV1 CgroupVersion = iota
	CgroupV2
)

// sudoCmd creates a command with privilege escalation based on SUDO_CMD env var.
// SUDO_CMD can be set to: "sudo" (default), "doas", "" (empty for root users), etc.
func sudoCmd(name string, args ...string) *exec.Cmd {
	if sudo := os.Getenv("SUDO_CMD"); sudo != "" {
		allArgs := append([]string{name}, args...)
		return exec.Command(sudo, allArgs...)
	}
	return exec.Command(name, args...)
}

// sudoCmdContext creates a command with privilege escalation and context.
func sudoCmdContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	if sudo := os.Getenv("SUDO_CMD"); sudo != "" {
		allArgs := append([]string{name}, args...)
		return exec.CommandContext(ctx, sudo, allArgs...)
	}
	return exec.CommandContext(ctx, name, args...)
}

// DetectCgroupVersion detects the cgroup version of the host
func DetectCgroupVersion() CgroupVersion {
	// Check if cgroup2 unified mode
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return CgroupV2
	}
	return CgroupV1
}

// ContainerdTestEnv manages a containerd instance for testing
type ContainerdTestEnv struct {
	t              *testing.T
	rootDir        string
	containerdCmd  *exec.Cmd
	socketPath     string
	pid            int
	cgroupVersion  CgroupVersion
	logFile        *os.File
	xfsImagePath   string
	xfsMountPath   string
	containerdPath string // Path to containerd binary
}

// NewContainerdTestEnv creates a new test environment
func NewContainerdTestEnv(t *testing.T) *ContainerdTestEnv {
	t.Helper()

	// Create temporary directory for test in current directory
	rootDir := filepath.Join(os.TempDir(), "nri-test", fmt.Sprintf("test-%d", time.Now().UnixNano()))

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	return &ContainerdTestEnv{
		t:              t,
		rootDir:        rootDir,
		cgroupVersion:  DetectCgroupVersion(),
		containerdPath: "containerd", // Default to PATH lookup
	}
}

// NewContainerdTestEnvWithPath creates a new test environment with specified containerd path
func NewContainerdTestEnvWithPath(t *testing.T, containerdPath string) *ContainerdTestEnv {
	t.Helper()

	// Create temporary directory for test in current directory
	rootDir := filepath.Join(os.TempDir(), "nri-test", fmt.Sprintf("test-%d", time.Now().UnixNano()))

	if err := os.MkdirAll(rootDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	return &ContainerdTestEnv{
		t:              t,
		rootDir:        rootDir,
		cgroupVersion:  DetectCgroupVersion(),
		containerdPath: containerdPath,
	}
}

// CgroupVersion returns the detected cgroup version
func (e *ContainerdTestEnv) CgroupVersion() CgroupVersion {
	return e.cgroupVersion
}

// IsCgroupV2 returns true if using cgroup v2
func (e *ContainerdTestEnv) IsCgroupV2() bool {
	return e.cgroupVersion == CgroupV2
}

// Setup starts containerd with test configuration
func (e *ContainerdTestEnv) Setup() error {
	e.t.Helper()

	// Create necessary directories
	dirs := []string{
		filepath.Join(e.rootDir, "containerd", "root"),
		filepath.Join(e.rootDir, "containerd", "state"),
		filepath.Join(e.rootDir, "containerd", "snapshots"),
		filepath.Join(e.rootDir, "nri", "conf.d"),
		filepath.Join(e.rootDir, "nri", "plugins"),
		filepath.Join(e.rootDir, "nri", "logs"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Setup XFS filesystem for containerd rootdir
	if err := e.setupXfsFilesystem(); err != nil {
		return fmt.Errorf("failed to setup XFS filesystem: %w", err)
	}

	// Generate containerd config
	configPath := filepath.Join(e.rootDir, "config.toml")
	if err := e.generateConfig(configPath); err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	// Start containerd
	e.socketPath = filepath.Join(e.rootDir, "containerd", "containerd.sock")

	// Setup containerd log file
	logFile := filepath.Join(e.rootDir, "containerd", "containerd.log")
	logFd, err := os.Create(logFile)
	if err != nil {
		return fmt.Errorf("failed to create containerd log file: %w", err)
	}
	e.logFile = logFd

	cmd := sudoCmd(e.containerdPath,
		"--config", configPath,
		"--address", e.socketPath,
		"--root", filepath.Join(e.rootDir, "containerd", "root"),
		"--state", filepath.Join(e.rootDir, "containerd", "state"),
	)

	cmd.Stdout = logFd
	cmd.Stderr = logFd

	if err := cmd.Start(); err != nil {
		e.printContainerdLog()
		return fmt.Errorf("failed to start containerd: %w", err)
	}

	e.containerdCmd = cmd
	e.pid = cmd.Process.Pid

	// Wait for containerd to be ready
	if err := e.waitForReady(); err != nil {
		e.Stop()
		return err
	}

	return nil
}

// Stop stops containerd and cleans up
func (e *ContainerdTestEnv) Stop() {
	if e.containerdCmd != nil && e.containerdCmd.Process != nil {
		_ = e.containerdCmd.Process.Kill()
		_ = e.containerdCmd.Wait()
	}
	if e.logFile != nil {
		_ = e.logFile.Close()
	}
	e.cleanupXfsFilesystem()
}

// SocketPath returns the containerd socket path
func (e *ContainerdTestEnv) SocketPath() string {
	return e.socketPath
}

// RootDir returns the test root directory
func (e *ContainerdTestEnv) RootDir() string {
	return e.rootDir
}

// LogDir returns the log directory
func (e *ContainerdTestEnv) LogDir() string {
	return filepath.Join(e.rootDir, "nri", "logs")
}

// ConfigPath returns the containerd config file path
func (e *ContainerdTestEnv) ConfigPath() string {
	return filepath.Join(e.rootDir, "config.toml")
}

// Crictl executes a crictl command
func (e *ContainerdTestEnv) Crictl(ctx context.Context, args ...string) (string, string, error) {
	e.t.Helper()

	// Use full URL format for runtime endpoint
	socketURL := "unix://" + e.socketPath
	allArgs := append([]string{"--runtime-endpoint", socketURL, "--image-endpoint", socketURL}, args...)
	cmd := sudoCmdContext(ctx, "crictl", allArgs...)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// InstallPlugin installs a NRI plugin binary with configuration
func (e *ContainerdTestEnv) InstallPlugin(index int, name, binaryPath, configContent string) error {
	e.t.Helper()

	// Format plugin name as [index]-[name]
	pluginName := fmt.Sprintf("%02d-%s", index, name)

	// Ensure directories exist
	pluginsDir := filepath.Join(e.rootDir, "nri", "plugins")
	confDir := filepath.Join(e.rootDir, "nri", "conf.d")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}
	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("failed to create conf directory: %w", err)
	}

	// Copy binary
	destBin := filepath.Join(pluginsDir, pluginName)
	if err := copyFile(binaryPath, destBin); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(destBin, 0755); err != nil {
		return fmt.Errorf("failed to chmod binary: %w", err)
	}

	// Write config if provided (config file should not have index prefix)
	if configContent != "" {
		destConf := filepath.Join(confDir, name+".conf")
		if err := os.WriteFile(destConf, []byte(configContent), 0644); err != nil {
			return fmt.Errorf("failed to write config: %w", err)
		}
	}

	return nil
}

// SandboxOption contains options for creating a sandbox
type SandboxOption struct {
	Labels      map[string]string
	HostNetwork bool
}

// CreateSandbox creates a sandbox using crictl
func (e *ContainerdTestEnv) CreateSandbox(ctx context.Context, name string, opts *SandboxOption) (string, error) {
	e.t.Helper()

	if opts == nil {
		opts = &SandboxOption{}
	}

	// Determine network mode
	networkMode := 2 // Default: namespace mode
	if opts.HostNetwork {
		networkMode = 0 // Host network
	}

	sandboxConfig := fmt.Sprintf(`
{
  "log_directory": "%s",
  "metadata": {
    "name": "%s",
    "namespace": "default",
    "uid": "test-sandbox-%d"
  },
  "linux": {
    "security_context": {
      "namespace_options": {
        "network": %d
      }
    }
  }
}`, e.rootDir, name, time.Now().UnixNano(), networkMode)

	configPath := filepath.Join(e.rootDir, "sandbox.json")
	if err := os.WriteFile(configPath, []byte(sandboxConfig), 0644); err != nil {
		return "", fmt.Errorf("failed to write sandbox config: %w", err)
	}

	stdout, stderr, err := e.Crictl(ctx, "runp", configPath)
	if err != nil {
		return "", fmt.Errorf("failed to create sandbox: %w (stderr: %s)", err, stderr)
	}

	return strings.TrimSpace(stdout), nil
}

// CreateContainerOption contains options for creating a container
type CreateContainerOption struct {
	Name        string
	Image       string
	Command     []string
	MemoryLimit int64 // 0 means no limit
	CPUPeriod   int64
	CPUQuota    int64
	Annotations map[string]string
}

// CreateContainer creates a container in a sandbox
func (e *ContainerdTestEnv) CreateContainer(ctx context.Context, sandboxID string, opts CreateContainerOption) (string, error) {
	e.t.Helper()

	if opts.Image == "" {
		opts.Image = "ghcr.io/kcrow-io/busybox:1.36"
	}
	if len(opts.Command) == 0 {
		opts.Command = []string{"sleep", "3600"}
	}

	// Build resources section
	resources := ""
	if opts.MemoryLimit > 0 || opts.CPUQuota > 0 {
		resources = `"resources": {`
		parts := []string{}
		if opts.MemoryLimit > 0 {
			parts = append(parts, fmt.Sprintf(`"memory_limit_in_bytes": %d`, opts.MemoryLimit))
		}
		if opts.CPUQuota > 0 {
			parts = append(parts, fmt.Sprintf(`"cpu_quota": %d`, opts.CPUQuota))
			parts = append(parts, fmt.Sprintf(`"cpu_period": %d`, opts.CPUPeriod))
		}
		resources += strings.Join(parts, ",") + `}`
	}

	commandJSON := `["` + strings.Join(opts.Command, `","`) + `"]`

	containerConfig := fmt.Sprintf(`
{
  "metadata": {
    "name": "%s",
    "namespace": "default",
    "uid": "test-container-%d"
  },
  "image": {
    "image": "%s"
  },
  "command": %s,
  "log_path":"busybox.0.log",
  "linux": {
    %s
  }
}`, opts.Name, time.Now().UnixNano(), opts.Image, commandJSON, resources)

	configPath := filepath.Join(e.rootDir, "container.json")
	if err := os.WriteFile(configPath, []byte(containerConfig), 0644); err != nil {
		return "", fmt.Errorf("failed to write container config: %w", err)
	}
	sandboxconfigPath := filepath.Join(e.rootDir, "sandbox.json")
	stdout, stderr, err := e.Crictl(ctx, "create", "--with-pull", sandboxID, configPath, sandboxconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w (stderr: %s)", err, stderr)
	}

	return strings.TrimSpace(stdout), nil
}

// StartContainer starts a container
func (e *ContainerdTestEnv) StartContainer(ctx context.Context, containerID string) error {
	_, stderr, err := e.Crictl(ctx, "start", containerID)
	if err != nil {
		return fmt.Errorf("failed to start container: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// StopContainer stops a container
func (e *ContainerdTestEnv) StopContainer(ctx context.Context, containerID string) error {
	_, stderr, err := e.Crictl(ctx, "stop", containerID)
	if err != nil {
		return fmt.Errorf("failed to stop container: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// RemoveContainer removes a container
func (e *ContainerdTestEnv) RemoveContainer(ctx context.Context, containerID string) error {
	_, stderr, err := e.Crictl(ctx, "rm", containerID)
	if err != nil {
		return fmt.Errorf("failed to remove container: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// StopSandbox stops a sandbox
func (e *ContainerdTestEnv) StopSandbox(ctx context.Context, sandboxID string) error {
	_, stderr, err := e.Crictl(ctx, "stopp", sandboxID)
	if err != nil {
		return fmt.Errorf("failed to stop sandbox: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// RemoveSandbox removes a sandbox
func (e *ContainerdTestEnv) RemoveSandbox(ctx context.Context, sandboxID string) error {
	_, stderr, err := e.Crictl(ctx, "rmp", sandboxID)
	if err != nil {
		return fmt.Errorf("failed to remove sandbox: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// Restart restarts containerd with existing configuration
func (e *ContainerdTestEnv) Restart() error {
	e.t.Helper()
	e.Stop()
	return e.Setup()
}

// UpdatePluginConfig updates a plugin's configuration file
func (e *ContainerdTestEnv) UpdatePluginConfig(name, configContent string) error {
	e.t.Helper()
	confDir := filepath.Join(e.rootDir, "nri", "conf.d")
	destConf := filepath.Join(confDir, name+".conf")
	return os.WriteFile(destConf, []byte(configContent), 0644)
}

// ClearPluginLog clears the plugin log file
func (e *ContainerdTestEnv) ClearPluginLog(pluginName string) error {
	e.t.Helper()
	logPath := filepath.Join(e.rootDir, "nri", "logs", pluginName+".log")
	return os.WriteFile(logPath, []byte(""), 0644)
}

// ReadPluginLog reads the plugin log file
func (e *ContainerdTestEnv) ReadPluginLog(pluginName string) (string, error) {
	logPath := filepath.Join(e.rootDir, "nri", "logs", pluginName+".log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WaitForPluginLog waits for a specific message in plugin log
func (e *ContainerdTestEnv) WaitForPluginLog(pluginName, expected string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		logContent, err := e.ReadPluginLog(pluginName)
		if err == nil && strings.Contains(logContent, expected) {
			return nil
		}

		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("failed to read log before deadline: %w", err)
			}
			return fmt.Errorf("expected log message %q not found before deadline", expected)
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func (e *ContainerdTestEnv) generateConfig(configPath string) error {
	// Determine containerd-shim path based on containerd path
	shimPath := "containerd-shim-runc-v2" // Default to PATH lookup
	if e.containerdPath != "containerd" {
		// If containerd path is specified, derive shim path from same directory
		shimDir := filepath.Dir(e.containerdPath)
		shimPath = filepath.Join(shimDir, "containerd-shim-runc-v2")
	}

	config := fmt.Sprintf(`
version = 2
root = "%s"
state = "%s"

[grpc]
  address = "%s"

[plugins."io.containerd.snapshotter.v1.overlayfs"]
  root_path = "%s"

[plugins."io.containerd.grpc.v1.cri"]
  disable_tcp_service = true
  sandbox_image = "registry.k8s.io/pause:3.8"

[plugins."io.containerd.grpc.v1.cri".containerd]
  default_runtime_name = "runc"

[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
  runtime_type = "io.containerd.runc.v2"
  runtime_path = "%s"

[plugins."io.containerd.grpc.v1.cri".cni]
  bin_dir = "/opt/cni/bin"
  conf_dir = "/etc/cni/net.d"

[plugins."io.containerd.nri.v1.nri"]
  disable = false
  disable_connections = false
  plugin_path = "%s"
  plugin_config_path = "%s"
  plugin_registration_timeout = "5s"
  plugin_request_timeout = "2s"
  socket_path = "%s"

[debug]
  level = "info"
`,
		filepath.Join(e.rootDir, "containerd", "root"),
		filepath.Join(e.rootDir, "containerd", "state"),
		e.socketPath,
		filepath.Join(e.rootDir, "containerd", "snapshots"),
		shimPath,
		filepath.Join(e.rootDir, "nri", "plugins"),
		filepath.Join(e.rootDir, "nri", "conf.d"),
		filepath.Join(e.rootDir, "nri", "nri.sock"),
	)

	return os.WriteFile(configPath, []byte(config), 0644)
}

func (e *ContainerdTestEnv) waitForReady() error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			// Read and print containerd log before returning error
			e.printContainerdLog()
			return fmt.Errorf("containerd not ready after 30 seconds")
		}

		// Try to connect to socket
		if _, err := os.Stat(e.socketPath); err == nil {
			// Try a simple crictl command
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _, err := e.Crictl(ctx, "pods")
			cancel()
			if err == nil {
				return nil
			}
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// printContainerdLog reads and prints the containerd log file for debugging
func (e *ContainerdTestEnv) printContainerdLog() {
	logPath := filepath.Join(e.rootDir, "containerd", "containerd.log")
	logContent, err := os.ReadFile(logPath)
	if err != nil {
		e.t.Logf("Failed to read containerd log file %s: %v", logPath, err)
		return
	}

	if len(logContent) == 0 {
		e.t.Logf("Containerd log file is empty (%s)", logPath)
		return
	}

	e.t.Logf("Containerd log content (%s):\n%s", logPath, string(logContent))
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0755)
}

// setupXfsFilesystem creates a 200MB XFS filesystem and mounts it to containerd rootdir
func (e *ContainerdTestEnv) setupXfsFilesystem() error {
	e.t.Helper()

	// Create 200MB empty file
	e.xfsImagePath = filepath.Join(e.rootDir, "xfs.img")
	cmd := exec.Command("dd", "if=/dev/zero", "of="+e.xfsImagePath, "bs=1M", "count=500")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create XFS image file: %w, output: %s", err, string(output))
	}

	// Format as XFS
	if output, err := sudoCmd("mkfs.xfs", "-f", e.xfsImagePath).CombinedOutput(); err != nil {
		return fmt.Errorf("failed to format XFS filesystem: %w, output: %s", err, string(output))
	}

	// Setup loop device using losetup for better compatibility
	output, err := sudoCmd("losetup", "-f", "--show", e.xfsImagePath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to setup loop device: %w, output: %s", err, string(output))
	}
	loopDevice := strings.TrimSpace(string(output))
	if loopDevice == "" {
		return fmt.Errorf("losetup returned empty device path")
	}

	// Mount to containerd rootdir
	e.xfsMountPath = filepath.Join(e.rootDir, "containerd", "root")
	if output, err := sudoCmd("mount", loopDevice, e.xfsMountPath).CombinedOutput(); err != nil {
		// Cleanup loop device on failure
		_, _ = sudoCmd("losetup", "-d", loopDevice).CombinedOutput()
		return fmt.Errorf("failed to mount XFS filesystem: %w, output: %s", err, string(output))
	}

	return nil
}

// cleanupXfsFilesystem unmounts and removes the XFS filesystem
func (e *ContainerdTestEnv) cleanupXfsFilesystem() {
	e.t.Helper()

	if e.xfsMountPath != "" {
		_, _ = sudoCmd("umount", e.xfsMountPath).CombinedOutput()
	}
	if e.xfsImagePath != "" {
		_ = os.Remove(e.xfsImagePath)
	}
}
