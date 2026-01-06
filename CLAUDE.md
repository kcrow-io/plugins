# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go-based NRI (Node Resource Interface) plugins collection for extending containerd's container runtime capabilities. The project provides specialized plugins that can modify container behavior at runtime through the containerd NRI framework.

## Development Commands

### Building
```bash
# Build all plugins for all platforms (linux/amd64, linux/arm64)
make build

# Build with vendor dependencies
make vendor

# Build container image
make image
```

### Testing and Linting
```bash
# Run Go linting
make lint-go

# Run tests (uses go test with vendor mode)
go test -mod=vendor ./...

# Run with race detection
RACE=1 make build
```

### Cross-compilation
The build system supports cross-compilation for:
- `linux/amd64`
- `linux/arm64`

Binaries are output to `bin/{platform}/` directories.

## Architecture

### Plugin System
- **Common Interface**: All plugins implement the `Pluginer` interface in `plugins/pluginer.go:21`
- **NRI Integration**: Uses containerd's NRI stub for plugin lifecycle management
- **Configuration**: Plugins support JSON-based configuration via the `Configer` interface
- **Annotation-based Control**: Plugins respond to container annotations with prefix `io.kcrow.`

### Available Plugins

1. **Override Plugin** (`cmd/override/`, `plugins/override/`)
   - Overrides container configurations according to OCI spec config files
   - Handles rlimit settings, hooks, and runtime parameters

2. **Escape Plugin** (`cmd/escape/`, `plugins/escape/`)
   - Allows containers to escape resource limits (CPU, memory)
   - Controlled via annotation: `io.kcrow.escape: cpu,memory`

3. **Memory Plugin** (`cmd/memory/`, `plugins/memory/`)
   - Automatically sets `memory.high` to a percentage of container's memory limit
   - Supports namespace filtering (include/exclude lists)
   - Configurable high percentage (default: 80%)

4. **IOLimit Plugin** (`cmd/iolimit/`, `plugins/iolimit/`)
   - Monitors container disk usage and automatically applies I/O bandwidth limits
   - Applies limits when disk usage exceeds configured threshold
   - Supports both cgroup v1 and v2
   - Configurable disk threshold, bandwidth limit, and watch interval

### Key Directories
- `cmd/`: Main entry points for each plugin
- `pkg/`: Shared utilities (annotation, cgroup, log)
- `deploy/`: Kubernetes deployment manifests
- `install/`: Installation scripts

### Build Configuration
- **GoReleaser**: Creates DEB/RPM packages, installs to `/opt/nri/bin`
- **Multi-stage Dockerfile**: Supports multi-arch builds
- **Makefile system**: Uses `Makefile.defs` for build configuration

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

# Test iolimit plugin (check that I/O limits are applied when disk usage exceeds threshold)
# Note: This requires the container to actually use disk space beyond the configured threshold
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  docker.io/library/alpine:latest test-iolimit sh -c "dd if=/dev/zero of=/tmp/testfile bs=1M count=5000"
```

## Code Patterns

### Plugin Implementation
1. Implement the `Pluginer` interface
2. Use `plugins.RunStub()` to start the NRI stub
3. Handle container lifecycle events through NRI callbacks
4. Use structured logging with logrus and the `pkg/log` package

### Configuration Management
- Plugins use disk-based storage for state management
- Configuration follows OCI spec compatibility
- JSON-based configuration with the `Configer` interface

### Annotation Processing
- All plugin annotations use the prefix `io.kcrow.`
- Use `pkg/annotation` utilities for consistent annotation handling