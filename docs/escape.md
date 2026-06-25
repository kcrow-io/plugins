# Escape Guard Plugin

The escape plugin detects containers whose init process has escaped to the host's root cgroup and periodically cleans up orphaned processes in those containers' mount namespaces.

## Features

- **Cgroupv2 Only**: Requires cgroup v2 unified hierarchy
- **PostStart Detection**: Monitors containers at start time for cgroup escape
- **Periodic Cleanup**: Regularly checks escaped containers and cleans up processes when containers are gone
- **CRI Integration**: Uses CRI API to query container status and detect when containers have been removed
- **Mount Namespace Cleanup**: Finds and terminates all processes sharing the same mount namespace as the escaped container
- **Graceful Termination**: Sends SIGTERM first, waits 3 seconds, then SIGKILL if needed

## Configuration

### Configuration Options

```json
{
  "log_path": "/var/log/escape-guard/audit.log",
  "cri_socket": "unix:///run/containerd/containerd.sock",
  "cleanup_interval": 30
}
```

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `log_path` | string | `""` | Path to audit log file |
| `cri_socket` | string | `"unix:///run/containerd/containerd.sock"` | CRI socket path for querying container status |
| `cleanup_interval` | int | `30` | Interval in seconds between cleanup checks |

## How it Works

The plugin operates as an NRI extension, registering for `PostStartContainer` events and running a background cleanup goroutine.

### Initialization Phase (Configure)

1. Parse configuration from JSON file
2. Detect cgroup version (requires cgroupv2)
3. Initialize CRI client connection
4. Perform initial container status sync via `syncContainerStatus()`

### PostStartContainer Callback

When a container starts:

1. **Skip if not cgroupv2**: Exit immediately if system is not using cgroup v2
2. **Get container init PID**: Extract PID from container metadata
3. **Check cgroup escape**:
   - If container has an expected cgroup path, use `cgroup.IsEscapedFromCgroup()` to compare
   - Fallback: use `cgroup.IsInRootCgroup()` to check if in root cgroup
4. **Track escaped container**: If escaped, store container ID with init PID and mount namespace inode

### Background Sync and Cleanup

The plugin runs background goroutines for ongoing monitoring:

#### syncContainerStatus()

Called once during initialization:

1. List all containers via CRI API (with retry on failure)
2. For each running container:
   - Get container PID from status response
   - Check if container is in root cgroup
   - If escaped, store in `escapedContainers` map
3. Launch `cleanupEscapedContainers()` goroutine

#### cleanupEscapedContainers()

Runs periodically at `cleanup_interval`:

1. Copy the escaped containers map
2. For each escaped container:
   - Check if container still exists via CRI
   - If container still running, skip
   - If container is gone, find all processes with the same mount namespace
   - Send SIGTERM to each process, wait 3 seconds, then SIGKILL if still alive
   - Remove container from tracking map

### Why Mount Namespace Cleanup?

When a container's init process escapes to the root cgroup, the container may leave behind orphaned processes. These processes share the same mount namespace as the original container. By finding all processes with matching mount namespace inodes, we can reliably identify and clean up all descendant processes, even if they've been reparented to the host init.

## Installation

### Prerequisites

- Containerd with NRI support enabled
- System must use **cgroup v2** (unified hierarchy)
- Root access for reading `/proc` and sending signals

### Step-by-Step Installation

1. **Enable NRI in containerd**
   
   Edit `/etc/containerd/config.toml`:
   ```toml
   [plugins."io.containerd.nri.v1"]
     enable = true
     plugin_path = "/opt/nri/plugins"
     socket_path = "/var/run/nri/nri.sock"
   ```
   Restart containerd after changes.

2. **Build and install the plugin**
   ```bash
   make build
   sudo cp bin/linux/amd64/escape /opt/nri/plugins/08-escape
   sudo chmod +x /opt/nri/plugins/08-escape
   ```

3. **Create configuration**
   ```bash
   sudo mkdir -p /opt/nri/conf
   sudo tee /opt/nri/conf/escape.conf <<EOF
   {
     "log_path": "/var/log/escape-guard/audit.log",
     "cleanup_interval": 30
   }
   EOF
   ```

4. **Restart containerd**
   ```bash
   sudo systemctl restart containerd
   ```

5. **Verify plugin loading**
   ```bash
   sudo journalctl -u containerd -f | grep escape
   ```

## Behavior Examples

### Example 1: Container with Escaped Init Process

**Scenario**: A container starts and its init process immediately moves to the host's root cgroup.

**What happens**:
1. `PostStartContainer` fires, detects the escape
2. Container ID is stored in `escapedContainers` map with init PID and mount namespace
3. Warning logged: `"Container escaped from expected cgroup"`
4. Background cleanup goroutine monitors the container

**When container is removed**:
1. Cleanup goroutine detects container is no longer running via CRI
2. Finds all processes in the container's mount namespace
3. Sends SIGTERM, waits 3 seconds, sends SIGKILL to remaining processes
4. Logs: `"Container cleanup completed"`

### Example 2: Normal Container (No Escape)

**Scenario**: A container starts normally, init process stays in its assigned cgroup.

**What happens**:
1. `PostStartContainer` fires, escape check returns false
2. Container is not tracked
3. No further action taken

### Example 3: Container Already Gone at Cleanup

**Scenario**: Container with escaped init is deleted before cleanup runs.

**What happens**:
1. Cleanup goroutine runs at `cleanup_interval`
2. CRI query returns error or non-running state
3. Processes in mount namespace are found and terminated
4. Container removed from tracking map

## Audit Logging

When `log_path` is configured, the plugin logs:

```
Container escape detected:
  Container abc123 (runtime io.containerd.runc.v2) init PID 12345 escaped to root cgroup
  Expected cgroup: /kubepods/besteffort/pod-uid/abc123
  Actual cgroup: /

Cleanup actions:
  Container abc123: Found 3 processes to cleanup
  PID 12345 (myapp): Sending SIGTERM
  PID 12345 (myapp): Process still running, sending SIGKILL
  Container abc123 cleanup completed
```

## Uninstallation

```bash
sudo rm /opt/nri/plugins/08-escape
sudo rm /opt/nri/conf/escape.conf
sudo systemctl restart containerd
```

## Limitations

- **Cgroupv2 only**: Does not work on systems using cgroup v1
- **Cleanup delay**: Up to `cleanup_interval` seconds before escaped processes are cleaned
- **SIGTERM wait**: Fixed 3-second grace period (not configurable)
- **No dry-run mode**: Always performs cleanup actions
