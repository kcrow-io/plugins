# NRI Plugins Collection

This project provides common NRI plugins to extend containerd's container runtime capabilities.

## Available Plugins

1. [override plugin](./cmd/override/README.md)  
   Overrides container configurations according to ocispec config file, including rlimit settings, hooks, etc.

2. [escape plugin](./cmd/escape/README.md)  
   Allows container's main process to escape resource limits based on annotation `io.kcrow.escape: cpu,memory`

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
