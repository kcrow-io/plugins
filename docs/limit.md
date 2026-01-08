# Limit Plugin

The limit plugin is an NRI plugin that provides dual resource management capabilities:
1. **I/O Limiting**: Monitors container disk usage and automatically applies I/O bandwidth limits when disk usage exceeds a configured threshold
2. **Memory Cache Management**: Monitors container memory cache/RSS ratio and automatically clears cache when it exceeds configured thresholds

This helps prevent containers from consuming excessive disk I/O resources and manages memory cache efficiently.

## Features

### I/O Limiting
- **Automatic I/O Limiting**: Applies bandwidth limits when disk usage exceeds threshold
- **Dynamic Monitoring**: Continuously watches container disk usage via containerd snapshotter
- **Cgroup v1/v2 Support**: Works with both cgroup versions
- **Configurable Thresholds**: Customizable disk usage threshold and bandwidth limits
- **Overlayfs Support**: Monitors disk usage through containerd's overlayfs snapshotter

### Memory Cache Management
- **Automatic Cache Clearing**: Triggers cache reclaim when cache/RSS ratio is too high
- **Ratio-based Detection**: Uses cache-to-RSS ratio to identify excessive cache usage
- **Minimum Threshold**: Only acts when cache exceeds minimum size to avoid unnecessary operations
- **Cgroup v1/v2 Support**: Uses appropriate mechanism for each cgroup version
  - v2: Uses `memory.reclaim` for proactive reclaim
  - v1: Uses `memory.force_empty` to force cache eviction

## How it Works

### I/O Limiting Flow
1. **Background Monitoring**: The plugin runs a background watcher that periodically checks container disk usage
2. **Threshold Detection**: When a container's disk usage exceeds the configured threshold, the plugin applies I/O limits
3. **Limit Application**:
   - For cgroup v2: Uses `io.max` to set write bandwidth limits
   - For cgroup v1: Uses `blkio.throttle.write_bps_device` to set write bandwidth limits
4. **Device Detection**: Automatically detects the block device used by the containerd root directory
5. **Snapshotter Integration**: Reads disk usage from containerd's snapshot service (overlayfs only)

### Memory Cache Management Flow
1. **Memory Stats Parsing**: Reads `memory.stat` from container's cgroup
2. **Ratio Calculation**: Calculates cache/RSS ratio from memory statistics
   - For cgroup v2: Uses `file` (cache) and `anon` (RSS) keys
   - For cgroup v1: Uses `cache` and `rss` keys
3. **Threshold Check**: Triggers when BOTH conditions are met:
   - Cache size > `min_cache_bytes` (default: 512MB)
   - Cache/RSS ratio > `cache_rss_ratio` (default: 10)
4. **Cache Clearing**:
   - For cgroup v2: Writes cache size to `memory.reclaim` for proactive reclaim
   - For cgroup v1: Writes "1" to `memory.force_empty` to force cache eviction

## Configuration

The plugin accepts JSON configuration with the following options:

```json
{
  "containerd_config_path": "/etc/containerd/config.toml",
  "watch_interval": 60,
  "io": {
    "max_disk_bytes": 4294967296,
    "bps_limit": 2049
  },
  "memory": {
    "cache_rss_ratio": 10,
    "min_cache_bytes": 536870912
  }
}
```

### Configuration Options

#### Global Options
- `containerd_config_path`: Path to containerd config file (default: `/etc/containerd/config.toml`)
  - Used to parse containerd root directory and socket path
- `watch_interval`: Interval in seconds for checking container stats (default: 60 seconds)

#### I/O Limiting Options (`io` section)
- `max_disk_bytes`: Disk usage threshold in bytes (default: 4GB = 4294967296)
  - When container disk usage exceeds this, I/O limits are applied
- `bps_limit`: Write bandwidth limit in bytes per second (default: 2049 bytes/s ≈ 2KB/s)
  - Applied when disk usage exceeds threshold

#### Memory Cache Options (`memory` section)
- `cache_rss_ratio`: Minimum cache-to-RSS ratio to trigger cache clearing (default: 10)
  - Cache clearing triggers when: cache/RSS > this ratio
  - Higher values mean more tolerance for cache
- `min_cache_bytes`: Minimum cache size in bytes to trigger clearing (default: 512MB = 536870912)
  - Prevents clearing cache when it's already small
  - Both ratio AND size conditions must be met

## Installation

1. Build the plugin:
   ```bash
   make build
   ```

2. Copy the binary to the NRI plugins directory:
   ```bash
   sudo cp bin/linux/amd64/limit /opt/nri/plugins/04-limit
   ```

3. Create configuration file:
   ```bash
   sudo mkdir -p /opt/nri/conf
   cat > /tmp/limit.conf <<EOF
   {
     "watch_interval": 60,
     "io": {
       "max_disk_bytes": 4294967296,
       "bps_limit": 10485760
     },
     "memory": {
       "cache_rss_ratio": 10,
       "min_cache_bytes": 536870912
     }
   }
   EOF
   sudo mv /tmp/limit.conf /opt/nri/conf/limit.conf
   ```

4. Restart containerd:
   ```bash
   sudo systemctl restart containerd
   ```

## Example Scenarios

### Scenario 1: Development Environment
Prevent containers from filling up disk and manage memory cache:

```json
{
  "watch_interval": 30,
  "io": {
    "max_disk_bytes": 2147483648,
    "bps_limit": 5242880
  },
  "memory": {
    "cache_rss_ratio": 8,
    "min_cache_bytes": 268435456
  }
}
```
- Disk threshold: 2GB
- I/O limit: 5MB/s
- Cache ratio: 8 (more aggressive)
- Min cache: 256MB
- Check interval: 30 seconds

### Scenario 2: Production Environment
Protect production systems with conservative settings:

```json
{
  "watch_interval": 60,
  "io": {
    "max_disk_bytes": 10737418240,
    "bps_limit": 10485760
  },
  "memory": {
    "cache_rss_ratio": 15,
    "min_cache_bytes": 1073741824
  }
}
```
- Disk threshold: 10GB
- I/O limit: 10MB/s
- Cache ratio: 15 (more tolerant)
- Min cache: 1GB
- Check interval: 60 seconds

### Scenario 3: Memory-Intensive Workloads
Focus on memory cache management:

```json
{
  "watch_interval": 30,
  "io": {
    "max_disk_bytes": 5368709120,
    "bps_limit": 10485760
  },
  "memory": {
    "cache_rss_ratio": 5,
    "min_cache_bytes": 536870912
  }
}
```
- Disk threshold: 5GB
- I/O limit: 10MB/s
- Cache ratio: 5 (very aggressive cache clearing)
- Min cache: 512MB
- Check interval: 30 seconds

## Monitoring

The plugin logs detailed information about its operations:

```bash
# Watch plugin activity
sudo journalctl -u containerd -f | grep limit

# Check for applied I/O limits
sudo journalctl -u containerd | grep "Applied io limit"

# Check for memory cache clearing
sudo journalctl -u containerd | grep "memory exceeds"
```

### Log Messages

#### I/O Limiting
- **Plugin Start**: "Configuring plugin (runtime: X, version: X)"
- **Device Detection**: "Detected device number: X:X"
- **Limit Applied**: "Applied io limit to container X (cgroup: X, disk usage: X bytes > threshold: X bytes)"

#### Memory Cache Management
- **Cache Clearing**: "Container id X memory exceeds, rss(X), cache(X), ratio(X.XX)"
- **Clear Success**: Cache is reclaimed via memory.reclaim (v2) or memory.force_empty (v1)
- **Clear Failure**: "Failed to clear cache to container X (cgroup: X): error"

## Verification

### Verify I/O Limits

1. **Check Plugin Status**:
   ```bash
   sudo journalctl -u containerd | grep "limit.*configured"
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

### Verify Memory Cache Management

1. **Check Memory Stats**:
   ```bash
   # For cgroup v2
   sudo cat /sys/fs/cgroup${CGROUP_PATH}/memory.stat | grep -E "file|anon"

   # For cgroup v1
   sudo cat /sys/fs/cgroup/memory${CGROUP_PATH}/memory.stat | grep -E "cache|rss"
   ```

2. **Monitor Cache Clearing Events**:
   ```bash
   sudo journalctl -u containerd -f | grep "memory exceeds"
   ```

3. **Check Reclaim Activity** (cgroup v2):
   ```bash
   # memory.reclaim is write-only, but you can monitor memory.stat changes
   watch -n 1 "sudo cat /sys/fs/cgroup${CGROUP_PATH}/memory.stat | grep file"
   ```

## Use Cases

### Disk Space Protection
- **Prevent Disk Exhaustion**: Automatically limit I/O when containers use too much disk
- **Multi-tenant Environments**: Protect shared storage from individual container abuse
- **Cost Control**: Limit disk I/O costs in cloud environments

### Memory Management
- **Cache Pressure Relief**: Automatically clear excessive cache to free memory
- **OOM Prevention**: Reduce memory pressure before OOM killer triggers
- **Performance Optimization**: Prevent cache from dominating memory usage
- **Fair Resource Sharing**: Ensure containers don't monopolize page cache

### Performance Management
- **Fair Resource Sharing**: Ensure containers don't monopolize disk I/O or memory
- **QoS Enforcement**: Maintain quality of service by limiting excessive resource usage
- **Noisy Neighbor Prevention**: Prevent one container from affecting others

### Development and Testing
- **Resource Constraints**: Test applications under I/O and memory-constrained conditions
- **Failure Simulation**: Simulate resource limitations for testing
- **Capacity Planning**: Understand application behavior under resource limits

## Limitations

### I/O Limiting
- **Overlayfs Only**: Currently only supports overlayfs snapshotter
- **Write Limits Only**: Only limits write bandwidth, not read bandwidth
- **Snapshot-based**: Monitors snapshot disk usage, not real-time I/O
- **Single Device**: Assumes all containers use the same block device

### Memory Cache Management
- **No Removal**: Limits are applied but not automatically removed when conditions improve
- **Periodic Checks**: Uses polling, not real-time monitoring
- **Cache-only**: Only manages page cache, not other memory types
- **Ratio-based**: May not be optimal for all workload patterns

### General
- **Polling-based**: Both features use periodic polling (watch_interval)
- **Container-only**: Only monitors actual containers, not sandboxes
- **No Per-namespace Config**: Single global configuration for all containers

## Troubleshooting

### Common Issues

#### I/O Limiting Issues

1. **Limits Not Applied**:
   - Check if containers are using overlayfs snapshotter
   - Verify disk usage actually exceeds threshold
   - Check plugin logs for errors
   - Verify device detection succeeded

2. **Device Detection Failed**:
   - Verify containerd root directory exists and is accessible
   - Check file system permissions
   - Ensure block device is properly mounted

3. **Cgroup I/O Errors**:
   - Verify cgroup filesystem is mounted
   - Check if io.max (v2) or blkio.throttle.write_bps_device (v1) is available
   - Ensure plugin has permissions to modify cgroup files

#### Memory Cache Issues

1. **Cache Not Clearing**:
   - Check if both ratio AND size thresholds are met
   - Verify memory.stat is readable
   - Check for permission errors in logs
   - Ensure memory.reclaim (v2) or memory.force_empty (v1) is available

2. **Excessive Cache Clearing**:
   - Increase `cache_rss_ratio` to be more tolerant
   - Increase `min_cache_bytes` to avoid clearing small caches
   - Increase `watch_interval` to reduce check frequency

3. **Memory Stats Parsing Errors**:
   - Verify cgroup version detection is correct
   - Check memory.stat format matches expected keys
   - Look for parsing errors in logs

#### General Issues

1. **High CPU Usage**:
   - Increase watch_interval to reduce polling frequency
   - Check if there are too many containers to monitor

2. **Configuration Not Loaded**:
   - Verify config file path and permissions
   - Check containerd config parsing succeeded
   - Look for "Configuring plugin" message in logs

### Debug Information

Enable debug logging:
```bash
# Check plugin configuration
sudo journalctl -u containerd | grep "limit.*Configuring"

# Monitor watcher activity
sudo journalctl -u containerd -f | grep "limit"

# Check device detection
sudo journalctl -u containerd | grep "Detected device number"

# Monitor memory cache operations
sudo journalctl -u containerd -f | grep "memory exceeds"

# Check containerd config parsing
sudo journalctl -u containerd | grep "containerd.*root"
```

## Performance Impact

- **CPU Usage**: Minimal, periodic polling based on watch_interval
- **Memory Usage**: Low, maintains minimal state
- **I/O Impact**:
  - Only affects containers exceeding disk threshold
  - Cache clearing may cause temporary I/O spike
- **Latency**: Polling interval determines detection latency

## Security Considerations

- **Resource Protection**: Prevents disk space exhaustion and memory pressure attacks
- **Fair Sharing**: Ensures equitable resource distribution
- **Privilege Required**: Needs permissions to modify cgroup files
- **Configuration Access**: Protect configuration files from unauthorized modification
- **Cache Clearing Impact**: Cache clearing may affect container performance temporarily

## Technical Details

### Cgroup File Operations

#### I/O Limiting
- **Cgroup v2**: Writes to `io.max` with format `"major:minor wbps=limit"`
- **Cgroup v1**: Writes to `blkio.throttle.write_bps_device` with format `"major:minor limit"`

#### Memory Cache Clearing
- **Cgroup v2**: Writes cache size to `memory.reclaim` for proactive reclaim
- **Cgroup v1**: Writes "1" to `memory.force_empty` to force cache eviction

### Memory Statistics Keys
- **Cgroup v2**:
  - Cache: `file` key in memory.stat
  - RSS: `anon` key in memory.stat
- **Cgroup v1**:
  - Cache: `cache` key in memory.stat
  - RSS: `rss` key in memory.stat

### Container Filtering
- Only processes containers with label `io.cri-containerd.kind=container`
- Skips pod sandboxes and other non-container entities

## Future Enhancements

Potential improvements for future versions:
- Support for additional snapshotters (btrfs, zfs, etc.)
- Read bandwidth limiting
- Per-namespace or per-pod configuration
- Real-time monitoring instead of polling
- IOPS (I/O operations per second) limiting
- Configurable limit escalation strategies
- Automatic limit removal when conditions improve
- More sophisticated memory management policies
- Integration with container resource requests/limits
- Metrics export for monitoring systems
