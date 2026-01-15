# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based NRI (Node Resource Interface) plugins collection for extending containerd's container runtime capabilities. The project provides specialized plugins that can modify container behavior at runtime through the containerd NRI framework.

## Development Commands

### Building
```bash
# Build all plugins for all platforms (linux/amd64, linux/arm64)
make build

# Build specific plugin for specific platform
GOOS=linux GOARCH=amd64 go build -o bin/linux/amd64/limit ./cmd/limit

# Build with race detection (requires CGO)
RACE=1 make build

# Build without optimization (for debugging)
NOOPT=1 make build

# Build without stripping symbols
NOSTRIP=1 make build

# Build container image
make image
```

### Testing and Linting
```bash
# Run all linting, tests, and vulnerability checks
make ci

# Run Go linting (uses golangci-lint)
make lint

# Run tests
make test

# Run tests with module mode
go test -mod=mod ./...

# Run tests for specific package
go test -mod=mod ./pkg/plugins/limit/...

# Run tests with race detection
RACE=1 go test -mod=mod ./...

# Run tests with verbose output
go test -v -mod=mod ./...

# Run vulnerability detection
make govulncheck
```

### Cross-compilation
The build system supports cross-compilation for:
- `linux/amd64`
- `linux/arm64`

Binaries are output to `bin/{platform}/` directories.

Build flags are controlled via `Makefile.defs`:
- `RACE=1`: Enable race detection (requires CGO)
- `NOOPT=1`: Disable optimizations for debugging
- `NOSTRIP=1`: Keep debug symbols
- `LOCKDEBUG=1`: Enable lock debugging

## Architecture

### Plugin System
- **Common Interface**: All plugins implement the `Pluginer` interface in `pkg/plugins/pluginer.go:22`
- **NRI Integration**: Uses containerd's NRI stub for plugin lifecycle management via `plugins.RunStub()`
- **Configuration**: Plugins support JSON-based configuration via the `Configer` interface (pkg/plugins/pluginer.go:17)
- **Annotation-based Control**: Plugins respond to container annotations with prefix `io.kcrow.` (defined in pkg/plugins/pluginer.go:13)

### Plugin Lifecycle
1. Each plugin's `main.go` creates a new plugin instance (e.g., `limitplugin.New()`)
2. `plugins.RunStub()` initializes the NRI stub and starts the plugin
3. The plugin registers with containerd's NRI framework
4. Container lifecycle events trigger plugin callbacks (CreateContainer, UpdateContainer, etc.)
5. Plugins read configuration from `/opt/nri/conf/<plugin-name>.conf` or `/etc/nri/conf.d/<plugin-name>.conf`

### Available Plugins

1. **Override Plugin** (`cmd/override/`, `pkg/plugins/override/`)
   - Overrides container configurations according to OCI spec config files
   - Handles rlimit settings, hooks, and runtime parameters

2. **Escape Plugin** (`cmd/escape/`, `pkg/plugins/escape/`)
   - Allows containers to escape resource limits (CPU, memory)
   - Controlled via annotation: `io.kcrow.escape: cpu,memory`

3. **Memory Plugin** (`cmd/memory/`, `pkg/plugins/memory/`)
   - Automatically sets `memory.high` to a percentage of container's memory limit
   - Supports namespace filtering (include/exclude lists)
   - Configurable high percentage (default: 80%)

4. **Limit Plugin** (`cmd/limit/`, `pkg/plugins/limit/`)
   - Monitors container disk usage and automatically applies I/O bandwidth limits
   - Also monitors memory cache/RSS ratio and clears cache when needed
   - **I/O Limiting**: Applies limits when disk usage exceeds `max_disk_bytes` threshold
   - **Memory Management**: Clears cache when:
     - Container memory usage/limit ratio exceeds `pods-usage-percent` (default: 80%)
     - Cache is greater than `min-cache-bytes` (default: 512MB)
     - Cache/RSS ratio exceeds `cache-rss-ratio` (default: 10)
   - Supports both cgroup v1 and v2
   - Uses containerd client to watch container stats at configurable intervals
   - Configuration in `pkg/plugins/limit/config.go:82`

### Key Directories
- `cmd/`: Main entry points for each plugin (one subdirectory per plugin)
- `pkg/plugins/`: Plugin implementations
  - `pkg/plugins/pluginer.go`: Core interfaces (`Pluginer`, `Configer`)
  - `pkg/plugins/limit/`, `memory/`, `escape/`, `override/`: Individual plugin implementations
- `pkg/containerd/`: Containerd client and watcher utilities
- `pkg/cgroup/`: Cgroup utilities for v1/v2 detection and path normalization
- `pkg/log/`: Structured logging setup
- `deploy/`: Kubernetes deployment manifests
- `docs/`: Plugin-specific documentation

### Cgroup Handling
The project includes sophisticated cgroup path handling in `pkg/cgroup/`:
- **Version Detection**: Automatically detects cgroup v1 vs v2 (cached for performance)
- **Systemd Path Conversion**: Converts systemd-style paths (with colons) to filesystem paths
- **Path Normalization**: Handles both systemd and cgroupfs drivers
- **PID-based Path Resolution**: Reads actual cgroup paths from `/proc/[pid]/cgroup` for reliability in Kubernetes

### Build Configuration
- **GoReleaser**: Creates DEB/RPM packages, installs to `/opt/nri/bin`
  - Configuration in `.goreleaser.yml`
  - Builds 4 plugins: override, escape, memory, limit
  - Supports linux/amd64 and linux/arm64
- **Multi-stage Dockerfile**: Supports multi-arch builds
- **Makefile system**: Uses `Makefile.defs` for build configuration with support for CGO, race detection, and debug builds
  - `START_NUM` in Makefile controls plugin numbering (default: 06)
  - Plugins are numbered sequentially (06-memory, 07-limit, 08-escape)
  - Build output: `bin/{platform}/{number}-{plugin-name}`

### Module Management
```bash
# Download dependencies
make download

# Tidy go.mod and go.sum
make tidy

# Format code
make fmt
```

## Installation and Deployment

### Package Installation
```bash
# Debian/Ubuntu
sudo dpkg -i nri-plugins_*.deb

# RHEL/CentOS
sudo rpm -ivh nri-plugins_*.rpm
```

### Containerd Configuration
```bash
# Enable NRI in containerd
sudo mkdir -p /etc/containerd/conf.d
echo 'disabled_plugins = []' | sudo tee /etc/containerd/conf.d/enable-nri.toml
sudo systemctl restart containerd
```

### Kubernetes Deployment
Use the DaemonSet in `deploy/daemonset.yml` for cluster-wide deployment.

## Testing Plugins

```bash
# Test escape plugin
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --annotation io.kcrow.escape=cpu,memory \
  docker.io/library/alpine:latest test

# Test memory plugin (requires container with memory limit)
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --memory 1073741824 \
  docker.io/library/alpine:latest test-memory

# Test limit plugin (check that I/O limits are applied when disk usage exceeds threshold)
# Note: This requires the container to actually use disk space beyond the configured threshold
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  docker.io/library/alpine:latest test-limit sh -c "dd if=/dev/zero of=/tmp/testfile bs=1M count=5000"
```

## Code Patterns

### Plugin Implementation
1. Implement the `Pluginer` interface (requires `Name() string` method)
2. Optionally implement the `Configer` interface for configuration support
3. Use `plugins.RunStub()` to start the NRI stub in `main()`
4. Handle container lifecycle events through NRI callbacks (CreateContainer, UpdateContainer, RemoveContainer, etc.)
5. Use structured logging with logrus via `pkg/log` package

Example plugin structure:
```go
type MyPlugin struct {
    stub stub.Stub
    config *Config
}

func (p *MyPlugin) Name() string {
    return "my-plugin"
}

func main() {
    if err := plugins.RunStub(New()); err != nil {
        log.G(context.TODO()).WithError(err).Fatal("Failed to run plugin")
        os.Exit(1)
    }
}
```

### Configuration Management
- Plugins use JSON-based configuration files
- Configuration files are typically located in `/opt/nri/conf/` or `/etc/nri/conf.d/`
- Implement `ReadFrom(io.Reader)` and `WriteTo(io.Writer)` from the `Configer` interface
- Use `Parse()` method to parse and validate configuration
- Configuration is read during plugin initialization

**Limit Plugin Configuration Structure** (`pkg/plugins/limit/config.go`):
```json
{
  "disabled": false,
  "containerd_config_path": "/etc/containerd/config.toml",
  "watch_interval": 60,
  "io": {
    "max_disk_bytes": 10737418240,
    "bps_limit": 4194304,
    "iops_limit": 10
  },
  "memory": {
    "pods-usage-percent": 80,
    "cache-rss-ratio": 10.0,
    "min-cache-bytes": 536870912
  },
  "log_path": "/var/log/nri-limit.log"
}
```

Key configuration fields:
- `io.max_disk_bytes`: Disk usage threshold for applying I/O limits (0 = disabled)
- `io.bps_limit`: Bandwidth limit in bytes per second
- `io.iops_limit`: IOPS limit
- `memory.pods-usage-percent`: Memory usage/limit percentage threshold (0 = disabled)
- `memory.cache-rss-ratio`: Minimum cache/RSS ratio to trigger cache clearing
- `memory.min-cache-bytes`: Minimum cache size before clearing

### Annotation Processing
- All plugin annotations use the prefix `io.kcrow.` (defined in `pkg/plugins/pluginer.go:13`)
- Annotations are comma-separated values (separator defined in `pkg/plugins/pluginer.go:14`)
- Example: `io.kcrow.escape: cpu,memory`

### Cgroup Operations
When working with cgroups:
1. Use `cgroup.DetectCgroupVersion()` to detect v1 vs v2 (cached, safe for concurrent use)
2. Use `cgroup.NormalizeCgroupPath()` to handle systemd vs cgroupfs paths
3. Use `cgroup.GetCgroupPathFromPid()` to get the actual cgroup path from a process PID
4. Be aware that Kubernetes adds additional parent slices (e.g., `kubelet.slice/kubepods-burstable.slice/`)

### Containerd Integration
The `pkg/containerd/` package provides:
- **Containerd Client**: Connect to containerd socket and interact with containers
- **Config Parsing**: Parse containerd config to get root directory and socket path (`containerd.ParseContainerdConfig()`)
- **Container Watcher**: Watch container stats (disk usage, memory usage) at configurable intervals
  - Watcher runs in background goroutine, polling containers at specified intervals
  - Calls custom callback function for each container (e.g., `checkContainer` in limit plugin)
  - Used by limit plugin to periodically check and apply/remove limits based on usage

### Limit Plugin Architecture
The limit plugin (`pkg/plugins/limit/`) has two main monitoring functions:

1. **I/O Limiting** (`limitblkio` in `limit_linux.go:94`):
   - Monitors disk usage via containerd snapshot service
   - Applies I/O limits (BPS and IOPS) when usage exceeds threshold
   - Removes limits when usage drops below threshold
   - Tracks limited containers in memory map to avoid redundant operations

2. **Memory Cache Management** (`clearCache` in `limit_linux.go:170`):
   - Monitors container memory usage/limit ratio
   - Checks cache/RSS ratio from cgroup memory.stat
   - Clears cache when all conditions are met (usage percent, min cache bytes, cache/RSS ratio)
   - Uses `memory.force_empty` or similar mechanisms to reclaim cache