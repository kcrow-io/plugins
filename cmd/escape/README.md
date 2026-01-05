# Escape Plugin

The escape plugin is an NRI plugin that allows container processes to escape from specific cgroup resource limits. When a container is annotated with the escape annotation, the plugin moves the container's main process to a different cgroup hierarchy, effectively bypassing the original resource constraints.

## Features

- **CPU Limit Escape**: Move processes out of CPU cgroup limits
- **Memory Limit Escape**: Move processes out of memory cgroup limits
- **Selective Escape**: Choose which resource limits to escape from
- **Annotation-Based Control**: Simple annotation-driven configuration
- **Process Tracking**: Maintains state for proper cleanup on container removal
- **Cgroup v1/v2 Support**: Works with both cgroup versions

## How it Works

1. **Container Start**: When a container starts, the plugin checks for escape annotations
2. **Process Migration**: If escape annotation is found, the plugin:
   - Creates a new cgroup under the configured root (default: `/system.slice`)
   - Moves the container's main process to the new cgroup
   - Applies only the non-escaped subsystems to the new cgroup
3. **State Management**: Stores process information for cleanup
4. **Container Removal**: When container is removed, cleans up the escape cgroup

## Configuration

### Plugin Configuration

The plugin accepts a JSON configuration:

```json
{
  "root": "/system.slice"
}
```

- `root`: The cgroup root path where escaped processes will be placed (default: `/system.slice`)

### Container Annotation

Add the escape annotation to containers that should escape resource limits:

```yaml
annotations:
  io.kcrow.escape: "cpu,memory"
```

### Supported Escape Types

- `cpu` - Escape CPU limits (cgroups cpu subsystem)
- `memory` - Escape memory limits (cgroups memory subsystem)
- `all` - Escape all supported resource limits

### Annotation Examples

```yaml
# Escape CPU limits only
annotations:
  io.kcrow.escape: "cpu"

# Escape memory limits only
annotations:
  io.kcrow.escape: "memory"

# Escape both CPU and memory limits
annotations:
  io.kcrow.escape: "cpu,memory"

# Escape all supported limits
annotations:
  io.kcrow.escape: "all"
```

## Installation

1. Build the plugin:
   ```bash
   make build
   ```

2. Copy the binary to the NRI plugins directory:
   ```bash
   sudo cp bin/linux/amd64/escape /opt/nri/bin/02-escape
   ```

3. Create configuration file (optional):
   ```bash
   sudo mkdir -p /etc/nri/conf.d
   echo '{"root": "/system.slice"}' | sudo tee /etc/nri/conf.d/escape.json
   ```

4. Restart containerd:
   ```bash
   sudo systemctl restart containerd
   ```

## Usage Examples

### Docker/Containerd

```bash
# Create container with CPU escape
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --annotation io.kcrow.escape=cpu \
  docker.io/library/alpine:latest test-cpu

# Create container with memory escape
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --annotation io.kcrow.escape=memory \
  docker.io/library/alpine:latest test-memory

# Create container escaping both limits
sudo ctr run --rm --runtime io.containerd.runc.v2 \
  --annotation io.kcrow.escape=cpu,memory \
  docker.io/library/alpine:latest test-both
```

### Kubernetes

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: escape-test
  annotations:
    io.kcrow.escape: "cpu,memory"
spec:
  containers:
  - name: test
    image: alpine:latest
    command: ["sleep", "3600"]
    resources:
      limits:
        cpu: "100m"
        memory: "128Mi"
```

## Verification

After starting a container with escape annotation, you can verify the escape worked:

```bash
# Find the container process
PID=$(pgrep -f "your-container-process")

# Check the cgroup path
cat /proc/$PID/cgroup

# The process should be in the escape cgroup (e.g., /system.slice/container-id)
# instead of the original container cgroup
```

## Use Cases

- **Performance Testing**: Remove resource limits for benchmarking
- **Emergency Scaling**: Temporarily bypass limits during high load
- **System Processes**: Allow critical system containers to use more resources
- **Development/Debug**: Remove limits for troubleshooting performance issues
- **Privileged Workloads**: Allow specific workloads to access full system resources

## Security Considerations

- **Resource Exhaustion**: Escaped processes can consume unlimited resources
- **System Impact**: May affect other containers and system stability
- **Access Control**: Ensure only trusted containers can use escape annotations
- **Monitoring**: Monitor escaped processes for resource usage

## Limitations

- Only affects the main container process, not child processes
- Requires appropriate permissions to manipulate cgroups
- May not work with all container runtimes or cgroup configurations
- Escaped processes are still subject to system-wide limits

## Troubleshooting

### Common Issues

1. **Permission Denied**: Ensure the plugin has sufficient privileges to modify cgroups
2. **Cgroup Not Found**: Verify the root cgroup path exists and is accessible
3. **Process Not Moved**: Check container runtime cgroup configuration
4. **Cleanup Failures**: Verify the plugin can access the status directory (`/run/escape/status`)

### Debug Information

The plugin logs detailed information about:
- Annotation parsing
- Cgroup operations
- Process migration
- Cleanup operations

Check containerd logs for plugin output:
```bash
sudo journalctl -u containerd -f | grep escape
```