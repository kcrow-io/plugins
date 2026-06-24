# NRI Plugins Collection

This project provides common NRI plugins to extend containerd's container runtime capabilities.

## Available Plugins

1. [override plugin](./docs/override.md)
   Overrides container configurations according to ocispec config file, including rlimit settings, hooks, etc.

2. [escape plugin](./docs/escape.md)
   Detects containers whose init process has escaped to the host's root cgroup and cleans up orphaned processes when containers are removed (cgroupv2 only)

3. [memory plugin](./docs/memory.md)
   Automatically sets `memory.high` to a percentage of container's memory limit for better memory management

4. [limit plugin](./docs/limit.md)
   Monitors container disk usage and automatically applies I/O bandwidth limits when disk usage exceeds a configured threshold. Also monitors memory cache/RSS ratio and clears cache when root-level memory pressure is detected and cache exceeds configured thresholds.

## Installation

### Prerequisites

- containerd >= 1.7.0 (with NRI support)

### NRI Configuration File Format

Each plugin requires a configuration file in JSON format, placed in `/opt/nri/conf/` directory. The configuration file should be named `<plugin-name>.conf`.

**Example configurations:**

**Memory Plugin** (`/opt/nri/conf/memory.conf`):
```json
{
  "include-namespace": [],
  "exclude-namespace": ["kube-system", "kube-public"],
  "high-ratio": 0.8
}
```

**Limit Plugin** (`/opt/nri/conf/limit.conf`):
```json
{
  "containerd_config_path": "/etc/containerd/config.toml",
  "io" : {
    "max_disk_bytes": 4294967296,
    "bps_limit": 4194304,
    "iops_limit": 10
  },
  "memory": {
    "pods-usage-percent": 80,
    "cache-rss-ratio": 10,
    "min-cache-bytes": 104857600
  },
  "watch_interval": 60
}
```

### Binary File Format

NRI plugins are standalone executables that implement the NRI protocol. They should be:

1. **Location**: Placed in `/opt/nri/plugins/` directory
2. **Naming**: Use descriptive names (e.g., `06-memory`, `07-limit`, `08-escape`)
   - The numeric prefix determines plugin execution order
3. **Permissions**: Must be executable (`chmod +x`)
4. **Format**: ELF 64-bit LSB executable for Linux

**Directory structure:**
```
/opt/nri/
└── plugins/
   ├── 06-memory      # Memory management plugin
   ├── 07-limit       # I/O limit plugin
   └── 08-escape      # Cgroup escape detection plugin
/etc/nri/
└── conf.d/
    ├── memory.conf    # Memory plugin configuration
    ├── limit.conf     # Limit plugin configuration
    └── escape.conf    # Escape plugin configuration
```

### Containerd Configuration

**Method 1: Enable NRI in containerd (Recommended)**

Edit `/etc/containerd/config.toml` and add:

```toml
version = 2

[plugins."io.containerd.nri.v1.nri"]
  disable = false
  disable_connections = false
  plugin_config_path = "/etc/nri/conf.d"
  plugin_path = "/opt/nri/plugins"
  plugin_registration_timeout = "5s"
  plugin_request_timeout = "2s"
  socket_path = "/var/run/nri/nri.sock"
```

**Method 2: Use drop-in configuration file**

```bash
# Create containerd drop-in directory
sudo mkdir -p /etc/containerd/conf.d

# Enable NRI plugin
cat <<EOF | sudo tee /etc/containerd/conf.d/enable-nri.toml
version = 2

[plugins."io.containerd.nri.v1.nri"]
  disable = false
  disable_connections = false
  plugin_config_path = "/etc/nri/conf.d"
  plugin_path = "/opt/nri/plugins"
EOF
```

### Installation Steps

**Option 1: Package Installation (Recommended)**

```bash
# 1. Install the package
# For Debian/Ubuntu:
sudo dpkg -i nri-plugins_*.deb

# For RHEL/CentOS:
sudo rpm -ivh nri-plugins_*.rpm

# 2. Configure containerd to enable NRI
sudo mkdir -p /etc/containerd/conf.d
cat <<EOF | sudo tee /etc/containerd/conf.d/enable-nri.toml
version = 2

[plugins."io.containerd.nri.v1.nri"]
  disable = false
  disable_connections = false
  plugin_config_path = "/etc/nri/conf.d"
  plugin_path = "/opt/nri/plugins"
EOF

# 3. Create plugin configuration directory
sudo mkdir -p /opt/nri/conf

# 4. Configure plugins (example for memory plugin)
cat <<EOF | sudo tee /opt/nri/conf/memory.conf
{
  "include-namespace": [],
  "exclude-namespace": ["kube-system", "kube-public"],
  "high-ratio": 0.8
}
EOF

# 5. Restart containerd
sudo systemctl restart containerd

# 6. Verify NRI is enabled
sudo ctr plugins ls | grep nri
```

**Option 2: Manual Installation**

```bash
# 1. Build plugins
make build

# 2. Create directories
sudo mkdir -p /opt/nri/plugins
sudo mkdir -p /opt/nri/conf

# 3. Copy plugin binaries
sudo cp bin/linux/amd64/memory /opt/nri/plugins/06-memory
sudo cp bin/linux/amd64/limit /opt/nri/plugins/07-limit
sudo cp bin/linux/amd64/escape /opt/nri/plugins/08-escape

# 4. Set executable permissions
sudo chmod +x /opt/nri/plugins/*

# 5. Configure containerd (see above)

# 6. Restart containerd
sudo systemctl restart containerd
```

### Verification

```bash
# Check if NRI plugin is loaded
sudo ctr plugins ls | grep nri

# Check containerd logs for NRI initialization
sudo journalctl -u containerd -f | grep -i nri

# Test with a container
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --memory-limit 1073741824 \
  docker.io/library/alpine:latest test sh -c "echo 'NRI plugin working'"
```

## Quick Start

```bash
# Build plugins
make build

# Create container with memory limits
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --memory-limit 1073741824 \
  docker.io/library/alpine:latest test
```

## Integration Tests

The `integration/` suite provisions a disposable KinD cluster, installs the limit plugin, and runs stress workloads to ensure disk I/O throttling and cache eviction paths behave as expected.

**Prerequisites**

- Docker engine (with BuildKit enabled)
- [kind](https://kind.sigs.k8s.io/) and `kubectl` binaries in `PATH`

**Run locally**

```bash
make e2e
```

The tests will build a local limit plugin image, load it into KinD, and assert on plugin logs for I/O throttling (`Applied io limit`) and memory cache clearing (`memory exceeds`). Use `GOFLAGS='-count=1' make e2e` when iterating to avoid test caching.

These checks also run in CI via `.github/workflows/e2e.yml`.

## Dependency Management

This project uses automated dependency management through GitHub Actions and Dependabot to keep dependencies up-to-date and secure.

### Automated Updates

- **Weekly Dependency Checks**: Every Monday at 9:00 AM Beijing time
- **Automatic PR Creation**: Creates PRs for dependency updates
- **Security Scanning**: Runs vulnerability checks on all dependencies
- **Auto-merge**: Automatically merges dependency PRs after CI passes

### Configuration Files

- `.github/dependabot.yml`: Dependabot configuration for Go modules, GitHub Actions, and Docker
- `.github/workflows/dependency-update.yml`: Advanced dependency checking and PR creation
- `.github/workflows/auto-merge-deps.yml`: Automatic merging of dependency PRs

### Manual Dependency Updates

```bash
# Check for outdated dependencies
go list -u -m all

# Update all dependencies to latest minor/patch versions
go get -u ./...

# Update to latest major versions (use with caution)
go get -u -t ./...

# Clean up and verify
go mod tidy
go mod verify

# Test after updates
make build
go test ./...
