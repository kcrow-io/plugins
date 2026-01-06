# VRuntime Plugin

The vruntime plugin is an NRI plugin that removes specific volume mounts from containers during creation. It is designed to enhance container security by automatically removing sensitive mount points, particularly those containing runtime secrets and service account tokens.

## Features

- **Automatic Mount Removal**: Removes mounts with destinations starting with `/var/run/secrets`
- **Security Enhancement**: Prevents containers from accessing sensitive runtime information
- **Zero Configuration**: Works out of the box without additional configuration
- **Kubernetes Integration**: Particularly useful for removing service account token mounts
- **Lightweight**: Minimal overhead and simple implementation

## How it Works

1. **Container Creation**: When a container is being created, the plugin examines all mount points
2. **Mount Filtering**: Identifies mounts with destinations starting with `/var/run/secrets`
3. **Mount Removal**: Removes matching mounts from the container configuration
4. **Container Start**: Container starts without the filtered mounts

## Target Mount Points

The plugin specifically targets mounts with destinations starting with:
- `/var/run/secrets/` - Common path for Kubernetes secrets and service account tokens

### Common Removed Mounts

In Kubernetes environments, this typically removes:
- `/var/run/secrets/kubernetes.io/serviceaccount/` - Service account tokens
- `/var/run/secrets/kubernetes.io/serviceaccount/token` - JWT tokens
- `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt` - CA certificates
- `/var/run/secrets/kubernetes.io/serviceaccount/namespace` - Namespace information

## Installation

1. Build the plugin:
   ```bash
   make build
   ```

2. Copy the binary to the NRI plugins directory:
   ```bash
   sudo cp bin/linux/amd64/vruntime /opt/nri/bin/03-vruntime
   ```

3. Restart containerd:
   ```bash
   sudo systemctl restart containerd
   ```

## Usage

The plugin works automatically once installed. No additional configuration or annotations are required.

### Verification

To verify the plugin is working, you can:

1. **Check Container Mounts**: Inspect a running container's mounts
   ```bash
   # Get container ID
   CONTAINER_ID=$(sudo ctr containers list | grep your-container | awk '{print $1}')

   # Check mounts (should not see /var/run/secrets mounts)
   sudo ctr containers info $CONTAINER_ID | grep -A 20 "Mounts"
   ```

2. **Inside Container**: Check if the paths exist
   ```bash
   # This should fail or show empty directory
   ls -la /var/run/secrets/
   ```

### Kubernetes Example

Before the plugin:
```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
spec:
  containers:
  - name: test
    image: alpine:latest
    command: ["sleep", "3600"]
    # Kubernetes automatically mounts service account token
```

After the plugin is installed, the service account token mount at `/var/run/secrets/kubernetes.io/serviceaccount/` will be automatically removed.

## Use Cases

### Security Hardening
- **Remove Service Account Tokens**: Prevent containers from accessing Kubernetes API
- **Secrets Isolation**: Block access to mounted secrets and sensitive information
- **Compliance**: Meet security requirements that prohibit runtime secret access

### Development and Testing
- **Clean Environment**: Ensure containers run without external dependencies
- **Isolation Testing**: Test applications without Kubernetes service integration
- **Security Testing**: Verify applications work without service account access

### Production Environments
- **Zero-Trust Architecture**: Implement principle of least privilege
- **Attack Surface Reduction**: Minimize available attack vectors
- **Audit Compliance**: Meet regulatory requirements for secret handling

## Security Considerations

### Benefits
- **Reduced Attack Surface**: Removes potential paths for privilege escalation
- **Secret Protection**: Prevents accidental or malicious access to sensitive data
- **Compliance**: Helps meet security standards and audit requirements

### Limitations
- **Application Compatibility**: Some applications may require service account access
- **Kubernetes Integration**: May break applications that depend on service discovery
- **Debugging**: Can make troubleshooting more difficult in some scenarios

## Compatibility

### Supported Environments
- **Kubernetes**: Works with all Kubernetes versions
- **Standalone Containerd**: Compatible with direct containerd usage
- **Container Runtimes**: Works with any OCI-compatible runtime

### Tested Scenarios
- Kubernetes Pods with service account tokens
- Containers with custom secret mounts
- Multi-container pods
- Init containers and sidecar containers

## Troubleshooting

### Common Issues

1. **Application Failures**: If applications fail after installing the plugin:
   - Check if the application requires service account access
   - Verify the application doesn't depend on files in `/var/run/secrets/`
   - Consider using alternative authentication methods

2. **Kubernetes API Access**: If pods can't access the Kubernetes API:
   - This is expected behavior - the plugin removes service account tokens
   - Use alternative authentication methods if API access is required
   - Consider using workload identity or external secret management

3. **Mount Still Present**: If mounts are not being removed:
   - Verify the plugin is installed and running
   - Check containerd logs for plugin errors
   - Ensure the mount path starts with `/var/run/secrets`

### Debug Information

Check containerd logs for plugin activity:
```bash
sudo journalctl -u containerd -f | grep vruntime
```

The plugin logs when it removes mounts, which can help verify it's working correctly.

## Configuration

The plugin currently has no configuration options and works with a fixed pattern (`/var/run/secrets`). Future versions may add:
- Configurable mount path patterns
- Whitelist/blacklist functionality
- Per-namespace or per-pod filtering

## Performance Impact

- **Minimal Overhead**: Simple string matching operation
- **No Runtime Impact**: Only affects container creation time
- **Memory Efficient**: No persistent state or memory usage
- **CPU Efficient**: Lightweight processing during container creation