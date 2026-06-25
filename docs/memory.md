# Memory Plugin

The memory plugin is an NRI plugin that automatically sets `memory.high` for containers based on their memory limits. It also sets the parent kubepods cgroup's memory limits on first container start.

## Features

- Automatically sets `memory.high` to a percentage of the container's memory limit
- Automatically sets kubepods parent cgroup `memory.high` (v2) or `memory.soft_limit_in_bytes` (v1) with retry on failure
- Supports namespace filtering (include/exclude lists)
- Configurable high ratio (default: 0.8, i.e., 80%)
- Cgroup v1 and v2 support with automatic detection
- Configurable file logging
- Plugin can be disabled via configuration

## Configuration

The plugin reads configuration from `/opt/nri/conf/memory.conf` or `/etc/nri/conf.d/memory.conf`.

### Configuration Options

```json
{
  "disabled": false,
  "include-namespace": ["production", "staging"],
  "exclude-namespace": ["kube-system", "kube-public"],
  "high-ratio": 0.8,
  "log_path": "/var/log/memory-plugin.log"
}
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `disabled` | bool | `false` | Disable the plugin entirely |
| `include-namespace` | []string | `[]` | Only process containers in these namespaces (empty = all) |
| `exclude-namespace` | []string | `[]` | Skip containers in these namespaces |
| `high-ratio` | float64 | `0.8` | Ratio of memory limit to set as memory.high (0.0-1.0) |
| `log_path` | string | `""` | Path to log file (empty = stdout only) |

## How it works

### Container Start Phase

When a container starts, the plugin performs the following steps:

1. **Disabled Check**: If `disabled` is true, skip processing entirely
2. **Namespace Filtering**: Check if the container's namespace matches filtering rules
3. **Kubepods Memory Setup** (first successful attempt):
   - Extract the kubepods cgroup path from the container's cgroups path
   - Read the kubepods memory limit (`memory.max` for v2, `memory.limit_in_bytes` for v1)
   - If the limit is `"max"` (unlimited), skip and retry later
   - Calculate target value: `limit * high_ratio`, capped at 200MB reduction
   - Write to `memory.high` (v2) or `memory.soft_limit_in_bytes` (v1)
   - On success, mark as done; on failure, retry on next container start
4. **Container Memory High**:
   - Skip if container has no memory limit configured
   - Calculate `memory_high = memory_limit * high_ratio`
   - Write to container's `memory.high` (v2) or `memory.soft_limit_in_bytes` (v1)

### Kubepods Memory High Behavior

The kubepods parent cgroup memory limit is set only once (on first successful attempt):

- **Success**: Sets the flag and skips future attempts
- **Failure with error**: Retries on next container start
- **Memory limit is "max"**: Retries later (does not set flag)
- **Calculated target < 0**: Skips permanently (sets flag)

## Cgroup Version Support

### Cgroup v2 (Unified Hierarchy)
- Container: Writes to `memory.high`
- Kubepods: Reads `memory.max`, writes to `memory.high`

### Cgroup v1 (Legacy)
- Container: Writes to `memory.soft_limit_in_bytes`
- Kubepods: Reads `memory.limit_in_bytes`, writes to `memory.soft_limit_in_bytes`

## Installation

1. Build the plugin:
   ```bash
   make build
   ```

2. Copy the binary to the NRI plugins directory:
   ```bash
   sudo cp bin/linux/amd64/memory /opt/nri/plugins/10-memory
   ```

3. Create configuration file:
   ```bash
   sudo mkdir -p /opt/nri/conf
   sudo tee /opt/nri/conf/memory.conf <<EOF
   {
     "high-ratio": 0.8,
     "log_path": "/var/log/memory-plugin.log"
   }
   EOF
   ```

4. Restart containerd:
   ```bash
   sudo systemctl restart containerd
   ```

## Example

For a container with memory limit of 1GB and high ratio of 0.8:
- Memory limit: 1073741824 bytes (1GB)
- Memory high: 858993459 bytes (~800MB)

This allows the container to use up to ~800MB before memory reclaim becomes more aggressive, while still enforcing the hard limit at 1GB.

## Troubleshooting

### Common Issues

1. **Container has no memory limit**
   - **Log message**: `"Container has no memory limit set, skipping"`
   - **Cause**: The container spec does not include `resources.limits.memory`
   - **Solution**: Set memory limits in the pod spec

2. **Failed to set kubepods memory limit**
   - **Log message**: `"Failed to set kubepods memory limit, will retry"`
   - **Cause**: Permission denied or cgroup path not found
   - **Solution**: Ensure the plugin runs with sufficient privileges; it will retry automatically

3. **Kubepods memory is 'max' (unlimited)**
   - **Log message**: `"kubepods memory.max is 'max' (unlimited), will retry later"`
   - **Cause**: The kubepods cgroup has no memory limit set yet
   - **Solution**: This is normal during early boot; the plugin will retry when containers start

### Debug Information

Check containerd logs for plugin output:
```bash
sudo journalctl -u containerd -f | grep memory
```

### Verification

To verify the plugin is working:

1. **Check Plugin Loading**:
   ```bash
   sudo journalctl -u containerd | grep "Memory plugin configured"
   ```

2. **Verify Container memory.high**:
   ```bash
   # For cgroup v2
   cat /sys/fs/cgroup/kubepods/<pod-uid>/<container-id>/memory.high
   
   # For cgroup v1
   cat /sys/fs/cgroup/memory/kubepods/<pod-uid>/<container-id>/memory.soft_limit_in_bytes
   ```

3. **Verify Kubepods memory.high**:
   ```bash
   # For cgroup v2
   cat /sys/fs/cgroup/kubepods/memory.high
   
   # For cgroup v1
   cat /sys/fs/cgroup/memory/kubepods/memory.soft_limit_in_bytes
   ```
