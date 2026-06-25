# E2E Testing Guide

This document describes how to run the end-to-end tests for the NRI plugins using crictl.

## Prerequisites

- **containerd** - Container runtime (v1.6+)
- **crictl** - Container runtime CLI tool (v1.26+)

### Installing crictl

```bash
VERSION="v1.26.1"
wget https://github.com/kubernetes-sigs/cri-tools/releases/download/$VERSION/crictl-$VERSION-linux-amd64.tar.gz
sudo tar zxvf crictl-$VERSION-linux-amd64.tar.gz -C /usr/local/bin
```

## Running Tests

### Run All Tests

```bash
make e2e
```

### Run Specific Plugin Tests

```bash
# Memory plugin tests
go test -v -tags e2e -run TestMemoryPlugin ./integration/

# Limit plugin tests
go test -v -tags e2e -run TestLimitPlugin ./integration/

# Cgroup version-specific tests
go test -v -tags e2e -run TestMemoryPluginCgroupVersions ./integration/
go test -v -tags e2e -run TestLimitPluginCgroupVersions ./integration/
```

### Run Specific Scenarios

```bash
# Run only default memory scenario
go test -v -tags e2e -run "TestMemoryPlugin/default_config" ./integration/

# Run only I/O scenarios
go test -v -tags e2e -run "TestLimitPlugin/IO" ./integration/

# Run only memory limit scenarios
go test -v -tags e2e -run "TestLimitPlugin/Memory" ./integration/
```

## Test Architecture

### How It Works

1. **Environment Setup**: Creates a temporary containerd instance with isolated storage
2. **Plugin Installation**: Copies plugin binaries and configuration
3. **Sandbox Creation**: Uses `crictl runp` to create pod sandbox
4. **Container Creation**: Uses `crictl create` with resource limits
5. **Verification**: Checks plugin logs for expected behavior
6. **Cleanup**: Destroys temporary containerd and all resources

### Cgroup Version Detection

Tests automatically detect the host's cgroup version:

- **cgroup v1**: Traditional hierarchy with separate controllers
- **cgroup v2**: Unified hierarchy with modern features

```go
// Auto-detected in test environment
env := testenv.NewContainerdTestEnv(t)
if env.IsCgroupV2() {
    // cgroup v2 specific logic
}
```

### Directory Structure

```
/tmp/nri-test/test-<timestamp>/
├── containerd/
│   ├── root/          # containerd root storage
│   ├── state/         # containerd state
│   └── containerd.sock
├── nri/
│   ├── conf.d/        # Plugin configurations
│   ├── plugins/       # Plugin binaries
│   ├── logs/          # Plugin logs
│   └── nri.sock       # NRI socket
└── config.toml        # containerd config
```

## Test Scenarios

### Memory Plugin Tests

| Scenario | Memory Limit | High Ratio | Expected Behavior |
|----------|--------------|------------|-------------------|
| default_config | 256MB | 0.8 | Sets memory.high |
| custom_ratio | 512MB | 0.5 | Sets memory.high with custom ratio |
| small_limit | 64MB | 0.9 | Sets memory.high for small limits |
| large_limit | 1GB | 0.7 | Sets memory.high for large limits |
| disabled_plugin | 256MB | 0.8 | Plugin disabled, logs skip message |
| no_memory_limit | 0 | 0.8 | No limit, logs skip message |

### Limit Plugin Tests

#### I/O Scenarios

| Scenario | Max Disk Bytes | BPS Limit | IOPS Limit | Expected Behavior |
|----------|---------------|-----------|------------|-------------------|
| default_io_config | 10MB | 1MB/s | 5 | Applies I/O limit |
| strict_io_limits | 5MB | 512KB/s | 2 | Applies strict I/O limit |
| disabled_io | 0 | 0 | 0 | Skips I/O limiting |

#### Memory Scenarios

| Scenario | Memory Limit | Pods Usage % | Cache-RSS Ratio | Expected Behavior |
|----------|--------------|--------------|-----------------|-------------------|
| default_memory_config | 256MB | 20% | 3.5 | Clears cache on pressure |
| low_memory_threshold | 128MB | 50% | 2.0 | Clears cache earlier |

## Adding New Test Scenarios

### 1. Add to Existing Test Function

```go
scenarios = append(scenarios, struct {
    name        string
    config      string
    memoryLimit int64
    expectedLog string
}{
    name: "new_scenario",
    config: `{...}`,
    memoryLimit: 512 * 1024 * 1024,
    expectedLog: "Expected log message",
})
```

### 2. Create New Test Function

```go
func TestNewPluginFeature(t *testing.T) {
    requireCommands(t, "containerd", "crictl")
    
    binPath := buildBinary(t, "pluginname")
    
    t.Run("scenario_name", func(t *testing.T) {
        env := testenv.NewContainerdTestEnv(t)
        // ... test logic
    })
}
```

### 3. Add Cgroup-Specific Tests

```go
func TestPluginCgroupVersions(t *testing.T) {
    env := testenv.NewContainerdTestEnv(t)
    
    t.Run(fmt.Sprintf("cgroup_v%d", env.CgroupVersion()+1), func(t *testing.T) {
        // Test implementation
    })
}
```

## Troubleshooting

### Permission Denied

```bash
# If containerd socket requires root
sudo go test -v -tags e2e ./integration/
```

### Privilege Escalation Configuration

The integration tests require root privileges for the following operations:
- **containerd** - Container runtime requires root access
- **crictl** - Container runtime CLI needs root to communicate with containerd
- **losetup** - Loop device setup for XFS filesystem
- **mount/umount** - Filesystem mounting operations

You can configure the privilege escalation command using the `SUDO_CMD` environment variable:

```bash
# Default: uses sudo
make e2e

# Use doas (alternative to sudo)
SUDO_CMD=doas make e2e

# No privilege escalation (when running as root)
SUDO_CMD="" make e2e

# Custom privilege command
SUDO_CMD="privilege-command" make e2e
```

In GitHub Actions, you can set this in the workflow:

```yaml
- name: Run E2E tests
  env:
    SUDO_CMD: "sudo"  # or "" if already root
  run: make e2e
```

### Containerd Won't Start

```bash
# Check if another containerd is running
ps aux | grep containerd

# Kill existing instances
sudo pkill containerd
```

### Plugin Not Found

```bash
# Ensure plugins are built
make build

# Check plugin binary
ls -la bin/linux/amd64/
```

### Test Timeout

Increase timeout:

```bash
go test -v -tags e2e -timeout 10m ./integration/
```

## CI/CD Integration

### GitHub Actions

```yaml
name: E2E Tests
on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Install crictl
        run: |
          VERSION="v1.26.1"
          wget https://github.com/kubernetes-sigs/cri-tools/releases/download/$VERSION/crictl-$VERSION-linux-amd64.tar.gz
          sudo tar zxvf crictl-$VERSION-linux-amd64.tar.gz -C /usr/local/bin
      - name: Run E2E tests
        run: make e2e
```

### Jenkins Pipeline

```groovy
pipeline {
    agent any
    stages {
        stage('E2E Tests') {
            steps {
                sh 'make e2e'
            }
        }
    }
}
```

## Performance

- **Test Duration**: 5-10 seconds per scenario
- **Isolation**: Each test creates independent containerd instance
- **Parallelization**: Tests run in parallel by default
- **Cleanup**: Automatic cleanup on test completion

## References

- [crictl Documentation](https://github.com/kubernetes-sigs/cri-tools)
- [containerd NRI](https://github.com/containerd/nri)
- [NRI Specification](https://github.com/containerd/nri/blob/main/doc/plugins.md)
