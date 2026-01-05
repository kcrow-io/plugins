# Memory Plugin

The memory plugin is an NRI plugin that automatically sets `memory.high` for containers based on their memory limits.

## Features

- Automatically sets `memory.high` to a percentage of the container's memory limit
- Supports namespace filtering (include/exclude lists)
- Configurable high percentage (default: 80%)
- **Cgroup v1 and v2 Support**: Automatically detects and works with both cgroup versions
- **Multiple Cgroup Drivers**: Supports both cgroupfs and systemd cgroup drivers
- **Robust Path Detection**: Automatically finds memory subsystem mount points
- Works with Kubernetes and standalone containerd

## Configuration

The plugin reads configuration from `/etc/nri/conf.d/memory.json` by default.

### Configuration Options

```json
{
  "include-namespace": ["production", "staging"],
  "exclude-namespace": ["kube-system", "kube-public"],
  "high": 0.8
}
```

- `include-namespace`: Only process containers in these namespaces (empty = all namespaces)
- `exclude-namespace`: Skip containers in these namespaces
- `high`: Percentage of memory limit to set as memory.high (0.0-1.0, default: 0.8)

## How it works

1. **Startup Detection**: At plugin startup, it detects the cgroup version (v1 or v2) and finds memory mount points
2. **Container Start**: When a container starts, the plugin checks if it has a memory limit set
3. **Namespace Filtering**: If the container's namespace matches the filtering rules, it proceeds
4. **Memory.high Calculation**: It calculates `memory.high` as `memory_limit * high_percentage`
5. **Direct Setting**: The plugin directly sets the `memory.high` value using the opencontainers/cgroups library
6. **Skip if No Limit**: If the container has no memory limit configured, the plugin skips processing

### Key Design Principles

- **One-time Detection**: Cgroup version and mount points are detected once at startup for efficiency
- **StartContainer Phase**: Memory.high is set during the container start phase, not creation
- **Skip No-limit Containers**: Containers without memory limits are automatically skipped
- **Minimal Overhead**: Simple and efficient implementation with minimal runtime checks

### Cgroup Version Support

#### Cgroup v2 (Unified Hierarchy)
- Uses `/sys/fs/cgroup` as the base path
- Directly writes to `memory.high` file
- Supports all modern container runtimes

#### Cgroup v1 (Legacy)
- Detects memory subsystem mount point (typically `/sys/fs/cgroup/memory`)
- Supports both cgroupfs and systemd drivers:
  - **systemd driver**: Handles `.slice` paths correctly
  - **cgroupfs driver**: Direct path mapping
- Gracefully handles systems where `memory.high` is not available

### Driver Detection

The plugin automatically detects the cgroup driver by examining the cgroup path:
- **systemd driver**: Paths contain `.slice` (e.g., `/system.slice/containerd.service/...`)
- **cgroupfs driver**: Direct hierarchical paths (e.g., `/kubepods/besteffort/...`)

## Installation

1. Build the plugin:
   ```bash
   make build
   ```

2. Copy the binary to the NRI plugins directory:
   ```bash
   sudo cp bin/linux/amd64/memory /opt/nri/bin/10-memory
   ```

3. Create configuration file:
   ```bash
   sudo mkdir -p /etc/nri/conf.d
   sudo cp cmd/memory/config.json /etc/nri/conf.d/memory.json
   ```

4. Restart containerd:
   ```bash
   sudo systemctl restart containerd
   ```

## Example

For a container with memory limit of 1GB and high percentage of 0.8:
- Memory limit: 1073741824 bytes (1GB)
- Memory high: 858993459 bytes (800MB)

This allows the container to use up to 800MB before memory reclaim becomes more aggressive, while still enforcing the hard limit at 1GB.

## Troubleshooting

### Common Issues

1. **Cgroup Path Not Found**
   - **Symptom**: Error "cgroup directory does not exist"
   - **Solution**: Verify the container runtime is properly configured and containers have valid cgroup paths

2. **Memory.high Not Available (Cgroup v1)**
   - **Symptom**: Warning "memory.high not available in cgroup v1, skipping"
   - **Explanation**: Some cgroup v1 systems don't support memory.high
   - **Solution**: This is expected behavior; the plugin will skip setting memory.high gracefully

3. **Permission Denied**
   - **Symptom**: Error writing to memory.high file
   - **Solution**: Ensure the plugin runs with sufficient privileges to modify cgroup files

4. **Mount Point Detection Failed**
   - **Symptom**: Error "memory cgroup mount point not found"
   - **Solution**: Verify cgroups are properly mounted and accessible

### Debug Information

The plugin logs detailed information about:
- Cgroup version detection (v1 vs v2)
- Cgroup driver detection (systemd vs cgroupfs)
- Path resolution and filesystem operations
- Memory.high calculations and settings

Check containerd logs for plugin output:
```bash
sudo journalctl -u containerd -f | grep memory
```

### Verification

To verify the plugin is working:

1. **Check Plugin Loading**:
   ```bash
   sudo journalctl -u containerd | grep "memory plugin configured"
   ```

2. **Verify Memory.high Setting**:
   ```bash
   # Find a container with memory limit
   CONTAINER_ID=$(sudo ctr containers list | grep your-container | awk '{print $1}')

   # Check the memory.high value (cgroup v2)
   sudo cat /sys/fs/cgroup/$(sudo ctr containers info $CONTAINER_ID | grep CgroupsPath | cut -d'"' -f4)/memory.high

   # For cgroup v1, check:
   sudo cat /sys/fs/cgroup/memory/$(sudo ctr containers info $CONTAINER_ID | grep CgroupsPath | cut -d'"' -f4)/memory.high
   ```

3. **Monitor Plugin Activity**:
   ```bash
   # Watch for memory plugin log entries
   sudo journalctl -u containerd -f | grep "io.kcrow.memory"
   ```