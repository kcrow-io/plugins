# NRI Plugins Collection

This project provides common NRI plugins to extend containerd's container runtime capabilities.

## Available Plugins

1. [override plugin](./cmd/override/README.md)
   Overrides container configurations according to ocispec config file, including rlimit settings, hooks, etc.

2. [escape plugin](./cmd/escape/README.md)
   Allows container's main process to escape resource limits based on annotation `io.kcrow.escape: cpu,memory`

3. [memory plugin](./cmd/memory/README.md)
   Automatically sets `memory.high` to a percentage of container's memory limit for better memory management

4. [vruntime plugin](./cmd/vruntime/README.md)
   Removes specific volume mounts (particularly `/var/run/secrets`) from containers for enhanced security

## Installation

```bash
# 1. Configure containerd to enable NRI
sudo mkdir -p /etc/containerd/conf.d
echo 'disabled_plugins = []' | sudo tee /etc/containerd/conf.d/enable-nri.toml

# 2. Copy release files according to your system
# For Debian/Ubuntu:
sudo dpkg -i nri-plugins_*.deb

# For RHEL/CentOS: 
sudo rpm -ivh nri-plugins_*.rpm

# 3. Restart containerd
sudo systemctl restart containerd
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
