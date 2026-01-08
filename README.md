# NRI Plugins Collection

This project provides common NRI plugins to extend containerd's container runtime capabilities.

## Available Plugins

1. [override plugin](./docs/override.md)
   Overrides container configurations according to ocispec config file, including rlimit settings, hooks, etc.

2. [escape plugin](./docs/escape.md)
   Allows container's main process to escape resource limits based on annotation `io.kcrow.escape: cpu,memory`

3. [memory plugin](./docs/memory.md)
   Automatically sets `memory.high` to a percentage of container's memory limit for better memory management

4. [limit plugin](./docs/limit.md)
   Monitors container disk usage and automatically applies I/O bandwidth limits when disk usage exceeds a configured threshold, clear cache when container cache usage exceeds a configured threshold

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
  "high": 0.8
}
```

**Limit Plugin** (`/opt/nri/conf/limit.conf`):
```json
{
  "containerd_config_path": "/etc/containerd/config.toml",
  "io" : {
    "max_disk_bytes": 4294967296, 
    "bps_limit": 1
  },
  "memory": {
    "cache-rss-ratio": 10,
    "min-cache-bytes": 104857600 
  },
  "watch_interval": 60
}
```

### Binary File Format

NRI plugins are standalone executables that implement the NRI protocol. They should be:

1. **Location**: Placed in `/opt/nri/plugins/` directory
2. **Naming**: Use descriptive names (e.g., `01-memory`, `02-escape`, `03-limit`)
   - The numeric prefix determines plugin execution order
3. **Permissions**: Must be executable (`chmod +x`)
4. **Format**: ELF 64-bit LSB executable for Linux

**Directory structure:**
```
/opt/nri/
└── plugins/
   ├── 01-memory      # Memory management plugin
   ├── 02-escape      # Resource limit escape plugin
   ├── 03-override    # Configuration override plugin
   └── 04-limit       # I/O limit plugin
/etc/nri/
└── conf.d/
    ├── memory.conf    # Memory plugin configuration
    └── limit.conf     # Limit plugin configuration
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
  "high": 0.8
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
sudo cp bin/linux/amd64/memory /opt/nri/plugins/01-memory
sudo cp bin/linux/amd64/escape /opt/nri/plugins/02-escape
sudo cp bin/linux/amd64/override /opt/nri/plugins/03-override
sudo cp bin/linux/amd64/limit /opt/nri/plugins/04-limit

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
  --annotation io.kcrow.escape=cpu,memory \
  docker.io/library/alpine:latest test sh -c "echo 'NRI plugin working'"
```

## Quick Start

```bash
# Build plugins
make build

# Create container with escape annotation
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --annotation io.kcrow.escape=cpu,memory \
  docker.io/library/alpine:latest test
```

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
