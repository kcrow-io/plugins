# IOLimit Plugin

The iolimit plugin is an NRI plugin that monitors container disk usage and automatically applies I/O bandwidth limits when disk usage exceeds a configured threshold. It helps prevent containers from consuming excessive disk I/O resources.

## Features

- **Automatic I/O Limiting**: Applies bandwidth limits when disk usage exceeds threshold
- **Dynamic Monitoring**: Continuously watches container disk usage
- **Automatic Removal**: Removes limits when disk usage drops below threshold
- **Cgroup v1/v2 Support**: Works with both cgroup versions
- **Configurable Thresholds**: Customizable disk usage threshold and bandwidth limits
- **Overlayfs Support**: Monitors disk usage through containerd's overlayfs snapshotter

## How it Works

1. **Background Monitoring**: The plugin runs a background watcher that periodically checks container disk usage
2. **Threshold Detection**: When a container's disk usage exceeds the configured threshold, the plugin applies I/O limits
3. **Limit Application**:
   - For cgroup v2: Uses `io.max` to set write bandwidth limits
   - For cgroup v1: Uses `blkio.throttle.write_bps_device` to set write bandwidth limits
4. **Automatic Cleanup**: When disk usage drops below the threshold, limits are automatically removed
5. **Device Detection**: Automatically detects the block device used by the containerd root directory

## Configuration

The plugin accepts JSON configuration with the following options:

```json
{
  "containerd_socket": "/run/containerd/containerd.sock",
  "containerd_root": "/var/lib/containerd",
  "max_disk_bytes": 4294967296,
  "bps_limit": 1024,
  "watch_interval": 10
}
```

### Configuration Options

- `containerd_socket`: Path to containerd socket (default: `/run/containerd/containerd.sock`)
- `containerd_root`: Containerd root directory (default: `/var/lib/containerd`)
- `max_disk_bytes`: Disk usage threshold in bytes (default: 4GB)
- `bps_limit`: Bandwidth limit in bytes per second when threshold is exceeded (default: 1KB/s)
- `watch_interval`: Interval in seconds for checking container disk usage (default: 10 seconds)

## Installation

1. Build the plugin:
   ```bash
   make build
   ```

2. Copy the binary to the NRI plugins directory:
   ```bash
   sudo cp bin/linux/amd64/iolimit /opt/nri/bin/20-iolimit
   ```

3. Create configuration file (optional):
   ```bash
   sudo mkdir -p /etc/nri/conf.d
   cat > /tmp/iolimit.json <<EOF
   {
     "max_disk_bytes": 4294967296,
     "bps_limit": 10485760,
     "watch_interval": 10
   }
   EOF
   sudo mv /tmp/iolimit.json /etc/nri/conf.d/iolimit.json
   ```

4. Restart containerd:
   ```bash
   sudo systemctl restart containerd
   ```

## Example Scenarios

### Scenario 1: Development Environment
Prevent containers from filling up disk during development:

```json
{
  "max_disk_bytes": 2147483648,
  "bps_limit": 5242880,
  "watch_interval": 5
}
```
- Threshold: 2GB
- Limit: 5MB/s
- Check interval: 5 seconds

### Scenario 2: Production Environment
Protect production systems from disk I/O abuse:

```json
{
  "max_disk_bytes": 10737418240,
  "bps_limit": 10485760,
  "watch_interval": 30
}
```
- Threshold: 10GB
- Limit: 10MB/s
- Check interval: 30 seconds

### Scenario 3: Strict Limits
Aggressive limiting for multi-tenant environments:

```json
{
  "max_disk_bytes": 1073741824,
  "bps_limit": 1048576,
  "watch_interval": 10
}
```
- Threshold: 1GB
- Limit: 1MB/s
- Check interval: 10 seconds

## Monitoring

The plugin logs detailed information about its operations:

```bash
# Watch plugin activity
sudo journalctl -u containerd -f | grep iolimit

# Check for applied limits
sudo journalctl -u containerd | grep "Applied io limit"

# Check for removed limits
sudo journalctl -u containerd | grep "Removed io limit"
```

### Log Messages

- **Plugin Start**: "Starting io watcher (cgroup version: vX, device: X:X, threshold: X bytes, limit: X bps, interval: X seconds)"
- **Limit Applied**: "Applied io limit to container X (disk usage: X bytes > threshold: X bytes)"
- **Limit Removed**: "Removed io limit from container X (disk usage: X bytes < threshold: X bytes)"

## Verification

To verify the plugin is working:

1. **Check Plugin Status**:
   ```bash
   sudo journalctl -u containerd | grep "iolimit.*configured"
   ```

2. **Monitor Container Disk Usage**:
   ```bash
   # Get container snapshot usage
   sudo ctr snapshots usage
   ```

3. **Check I/O Limits** (cgroup v2):
   ```bash
   # Find container cgroup path
   CONTAINER_ID=$(sudo ctr containers list | grep your-container | awk '{print $1}')
   CGROUP_PATH=$(sudo ctr containers info $CONTAINER_ID | grep CgroupsPath | cut -d'"' -f4)

   # Check io.max
   sudo cat /sys/fs/cgroup${CGROUP_PATH}/io.max
   ```

4. **Check I/O Limits** (cgroup v1):
   ```bash
   # Check blkio throttle
   sudo cat /sys/fs/cgroup/blkio${CGROUP_PATH}/blkio.throttle.write_bps_device
   ```

## Use Cases

### Disk Space Protection
- **Prevent Disk Exhaustion**: Automatically limit I/O when containers use too much disk
- **Multi-tenant Environments**: Protect shared storage from individual container abuse
- **Cost Control**: Limit disk I/O costs in cloud environments

### Performance Management
- **Fair Resource Sharing**: Ensure containers don't monopolize disk I/O
- **QoS Enforcement**: Maintain quality of service by limiting excessive I/O
- **Noisy Neighbor Prevention**: Prevent one container from affecting others

### Development and Testing
- **Resource Constraints**: Test applications under I/O-constrained conditions
- **Failure Simulation**: Simulate disk I/O limitations for testing
- **Capacity Planning**: Understand application behavior under I/O limits

## Limitations

- **Overlayfs Only**: Currently only supports overlayfs snapshotter
- **Write Limits Only**: Only limits write bandwidth, not read bandwidth
- **Snapshot-based**: Monitors snapshot disk usage, not real-time I/O
- **Periodic Checks**: Uses polling, not real-time monitoring
- **Single Device**: Assumes all containers use the same block device

## Troubleshooting

### Common Issues

1. **Limits Not Applied**:
   - Check if containers are using overlayfs snapshotter
   - Verify disk usage actually exceeds threshold
   - Check plugin logs for errors

2. **Device Detection Failed**:
   - Verify containerd root directory exists and is accessible
   - Check file system permissions
   - Ensure block device is properly mounted

3. **Cgroup Errors**:
   - Verify cgroup filesystem is mounted
   - Check if io.max (v2) or blkio.throttle.write_bps_device (v1) is available
   - Ensure plugin has permissions to modify cgroup files

4. **High CPU Usage**:
   - Increase watch_interval to reduce polling frequency
   - Check if there are too many containers to monitor

### Debug Information

Enable debug logging:
```bash
# Check plugin configuration
sudo journalctl -u containerd | grep "iolimit.*Configuring"

# Monitor watcher activity
sudo journalctl -u containerd -f | grep "io watcher"

# Check device detection
sudo journalctl -u containerd | grep "Detected device number"
```

## Performance Impact

- **CPU Usage**: Minimal, periodic polling based on watch_interval
- **Memory Usage**: Low, maintains small map of limited containers
- **I/O Impact**: Only affects containers exceeding threshold
- **Latency**: Polling interval determines detection latency

## Security Considerations

- **Resource Protection**: Prevents disk space exhaustion attacks
- **Fair Sharing**: Ensures equitable resource distribution
- **Privilege Required**: Needs permissions to modify cgroup files
- **Configuration Access**: Protect configuration files from unauthorized modification

## Future Enhancements

Potential improvements for future versions:
- Support for additional snapshotters (btrfs, zfs, etc.)
- Read bandwidth limiting
- Per-namespace or per-pod configuration
- Real-time I/O monitoring instead of polling
- IOPS (I/O operations per second) limiting
- Configurable limit escalation strategies
