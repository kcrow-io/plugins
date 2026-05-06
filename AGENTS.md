## Project Overview

This is a collection of NRI (Node Resource Interface) plugins for containerd, written in Go. The plugins extend containerd's container runtime capabilities for resource management, including memory limits, I/O throttling, and resource limit escaping.

## Build Commands

```bash
# Build all plugins (formats and tidies first)
make build

# Format code
make fmt

# Tidy dependencies
make tidy

# Run linter
make lint

# Run all unit tests
make test

# Run specific test
go test ./pkg/plugins/memory/...

# Run single test function
go test -run TestFunctionName ./path/to/package

# Run e2e tests (requires docker, kind, kubectl)
make e2e

# Avoid test caching during iteration
GOFLAGS='-count=1' make e2e

# Run vulnerability scan
make govulncheck

# Run full CI suite (lint + test + govulncheck)
make ci

# Build container image
make image
```

## Build Options

```bash
# Enable race detection (requires CGO)
RACE=1 make build

# Keep debug symbols (no stripping)
NOSTRIP=1 make build

# Disable optimizations (for debugging)
NOOPT=1 make build

# Build for specific platform
BUILD_PLATFORMS=linux/arm64 make build
```

## Architecture

### Plugin System

All plugins implement the NRI protocol via `github.com/containerd/nri/pkg/stub`. The common pattern:

1. Each plugin implements the `Pluginer` interface (`pkg/plugins/pluginer.go`)
2. Plugin main.go calls `plugins.RunStub()` with the plugin instance
3. Plugins communicate with containerd via the NRI socket
4. Configuration is loaded from JSON files via the `Configer` interface

### Directory Structure

- `cmd/` - Plugin binaries (memory, limit, escape, override, example)
- `pkg/plugins/` - Plugin implementations
- `pkg/cgroup/` - cgroup v2 utilities
- `pkg/containerd/` - containerd client and watcher
- `pkg/pool/` - Worker pool for concurrent operations
- `pkg/log/` - Logging utilities
- `integration/` - KinD-based e2e tests

### Plugin Execution Order

Plugins are named with numeric prefixes that determine execution order:
- `06-memory` (START_NUM=06 in Makefile)
- `07-limit`
- `08-escape`

### Key Conventions

- **Annotation prefix**: `io.kcrow.` (e.g., `io.kcrow.escape=cpu,memory`)
- **Config location**: `/opt/nri/conf/` or `/etc/nri/conf.d/`
- **Binary location**: `/opt/nri/plugins/`
- **Config format**: JSON files named `<plugin-name>.conf`

## Plugin Implementations

### memory
Automatically sets `memory.high` to a percentage of container's memory limit for better memory management.

### escape
Allows container's main process to escape resource limits based on annotation `io.kcrow.escape: cpu,memory`.

### limit
Monitors container disk usage and applies I/O bandwidth limits when thresholds are exceeded. Also monitors memory cache/RSS ratio and clears cache under memory pressure.

### override
Overrides container configurations according to ocispec config file, including rlimit settings and hooks.

## Testing

### Unit Tests
Located in `pkg/` subdirectories. Run with `make test` or `go test ./...`.

### E2E Tests
Located in `integration/`. These provision a KinD cluster, install plugins, and run stress workloads.

**Prerequisites**: docker, kind, kubectl in PATH

The e2e tests verify:
- Disk I/O throttling (looks for "Applied io limit" in logs)
- Memory cache clearing (looks for "memory exceeds" in logs)

## Development Notes

- **CGO**: Disabled by default (`CGO_ENABLED=0`) for static binaries
- **Cross-compilation**: Supports linux/amd64 and linux/arm64
- **Linting**: Uses golangci-lint v2 (imported as Go module dependency)
- **Build tags**: `osusergo` (always), `lockdebug` (if LOCKDEBUG set)
- **Binary stripping**: Enabled by default (use NOSTRIP=1 to disable)

## Adding a New Plugin

1. Create plugin implementation in `pkg/plugins/<name>/`
2. Implement the NRI stub interface methods
3. Create `cmd/<name>/main.go` that calls `plugins.RunStub()`
4. Add plugin to `BIN_SUBDIRS` in Makefile
5. Create configuration struct implementing `Configer` interface
6. Add documentation in `docs/<name>.md`
