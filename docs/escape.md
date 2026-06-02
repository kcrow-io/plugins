# Escape Guard Plugin

## Features

- **Cgroup Version Agnostic**: Works on both cgroup v1 and v2 systems (no requirement for cgroup v2). The detection logic relies on cgroup namespace inode comparison, which is available in both versions.
- **Runtime‑Specific Protection**: Only activates for containers using the `io.containerd.runc.v2` runtime, ignoring other runtimes to avoid unnecessary operations.
- **Root Cgroup Escape Detection**: Identifies when a container’s main (init) process has moved to the host’s root cgroup (any cgroup version) by comparing its cgroup namespace inode with that of the host PID 1.
- **Mount Namespace Affinity Cleanup**: After confirming the init process is in the root cgroup, the plugin finds all processes sharing the same mount namespace (mnt ns) as the container and cleans them up, preventing resource leaks.
- **Graceful Termination with Timeout**: Sends SIGTERM to all identified processes, waits for a configurable grace period, then forcibly SIGKILL any remaining processes.
- **Minimal Configuration**: Only one configuration item is required: `log_path` for audit logging. All other behaviors are built‑in with sensible defaults.

## How it Works

The plugin operates as an NRI (Node Resource Interface) extension, registering a callback for the `StopContainer` event. Its logic is executed when a container is being stopped (or deleted).

### Initialization Phase (Plugin Start)

1. The plugin registers itself with containerd’s NRI framework and subscribes to `StopContainer` events.
2. It retrieves the **cgroup namespace inode** of the host’s PID 1 process (i.e., `init` or `systemd`). This inode uniquely identifies the root cgroup namespace of the host. The inode is obtained via `/proc/1/ns/cgroup`.
   - This value is stored as a reference for later comparisons.

### StopContainer Callback Execution

When a container stop request arrives (e.g., `crictl stop` or pod deletion), the plugin’s callback is invoked with the container’s metadata.

1. **Runtime check** – The plugin examines the container’s runtime name. If it is **not** equal to `io.containerd.runc.v2`, the plugin returns immediately without any action.
2. **Obtain container init PID** – The plugin queries the container’s runtime state to get the PID of its init process (the main process inside the container).
3. **Check cgroup namespace escape** – It reads the cgroup namespace inode of the container’s init PID from `/proc/<pid>/ns/cgroup`.
   - If this inode is **different** from the host PID 1’s cgroup namespace inode, the process is still inside a container‑isolated cgroup namespace; the plugin returns (no escape).
   - If the inodes **match**, the init process has moved into the host’s root cgroup namespace – this is the “escaped to root cgroup” condition (applicable to both cgroup v1 and v2).
4. **Find all processes with the same mount namespace** – The plugin scans `/proc` and, for each process, reads its mount namespace inode (`/proc/<pid>/ns/mnt`). It collects all PIDs whose mount namespace inode equals that of the container’s init process. This captures every process that was part of the container’s mount namespace (and likely the entire container process tree), even if they have also leaked into the root cgroup.
5. **Execute cleanup** – For each collected PID:
   - It sends `SIGTERM` to the process.
   - It waits for a built‑in grace period (default 5 seconds, not configurable).
   - If the process still exists, it sends `SIGKILL`.
   - All actions are logged to the configured `log_path`.
6. The plugin then returns control to containerd. Once the orphaned processes are gone, the container’s shim can exit cleanly, and the container deletion completes without resource leaks.

### Why Compare Mount Namespace Instead of Process Tree?

Directly using parent PID (PPID) chains can be unreliable when processes have been reparented to init after the container’s init exits. The mount namespace is a stable identifier of the container’s original isolation boundary; any process that shares the same mount namespace originally belonged to that container. This method robustly catches all descendant processes, even if they have become orphans or have been reattached to a different parent.

## Configuration

The plugin is configured via a JSON file placed in the NRI configuration directory. The only supported configuration option is `log_path`, which defines where audit logs are written.

### Plugin Configuration

Create the file `/etc/nri/conf.d/escape.conf` with the following content:

```json
{
  "log_path": "/var/log/escape-guard/audit.log"
}
```

- **`log_path`** (string, required): Absolute path to the audit log file. The plugin creates the file if it does not exist, and it logs every cleanup action with timestamps, affected PIDs, signals sent, and final status.  
  Example log entry:
  ```
  2025-05-29T14:22:10Z [INFO] Container abc123 (runtime io.containerd.runc.v2) init PID 12345 escaped to root cgroup
  2025-05-29T14:22:10Z [INFO] Sending SIGTERM to PID 67890 (bash)
  2025-05-29T14:22:15Z [INFO] PID 67890 still running, sending SIGKILL
  2025-05-29T14:22:15Z [INFO] Cleanup completed: 3 processes terminated
  ```

All other behaviors are hardcoded and not user‑adjustable:
- Grace period between SIGTERM and SIGKILL: **5 seconds**
- Cleanup timeout (maximum time spent killing): **30 seconds** (the plugin will not block containerd indefinitely)
- The plugin always forces `SIGKILL` after the grace period; no option to disable it.
- No dry‑run mode.

## Installation

### Prerequisites

- Containerd version ≥ 1.6 with NRI compiled in (most distributions enable it by default)
- The runtime `io.containerd.runc.v2` must be available and used by the containers to be protected
- Root access on the node (to read `/proc` and send signals)

### Step‑by‑Step Installation

1. **Enable NRI in containerd**  
   Edit `/etc/containerd/config.toml` and ensure the following section exists:
   ```toml
   [plugins."io.containerd.nri.v1"]
     enable = true
     plugin_path = "/opt/nri/plugins"
     socket_path = "/var/run/nri/nri.sock"
   ```
   If the section is missing, add it and restart containerd.

2. **Create directories**
   ```bash
   sudo mkdir -p /opt/nri/plugins
   sudo mkdir -p /etc/nri/conf.d
   sudo mkdir -p $(dirname /var/log/escape-guard)
   ```

3. **Place the plugin binary**  
   Build or download the `escape-guard` binary and copy it to `/opt/nri/plugins/08-escape`:
   ```bash
   sudo cp escape-guard /opt/nri/plugins/08-escape
   sudo chmod +x /opt/nri/plugins/08-escape
   ```

4. **Create the configuration file**  
   Write the JSON configuration (only `log_path`) to `/etc/nri/conf.d/escape.conf`:
   ```bash
   sudo tee /etc/nri/conf.d/escape.conf <<EOF
   {
     "log_path": "/var/log/escape-guard/audit.log"
   }
   EOF
   ```

5. **Restart containerd**
   ```bash
   sudo systemctl restart containerd
   ```

6. **Verify the plugin is loaded**  
   Check containerd logs:
   ```bash
   sudo journalctl -u containerd -f | grep -i "08-escape"
   ```
   You should see a line like `08-escape plugin registered for StopContainer events`.

### Uninstallation

- Remove the plugin binary: `sudo rm /opt/nri/plugins/08-escape`
- Remove the configuration file: `sudo rm /etc/nri/conf.d/escape.conf`
- Restart containerd: `sudo systemctl restart containerd`

## Usage Examples

### Example 1: Container with Init Process That Escapes to Root Cgroup (cgroup v2 or v1)

**Scenario**: A container running with `io.containerd.runc.v2` is started. The user manually moves its init process to the host’s root cgroup (e.g., by writing its PID to `/sys/fs/cgroup/cgroup.procs`). The container then stops, but the init process remains as an orphan in the root cgroup.

**Outcome**:
- The plugin checks the runtime: `io.containerd.runc.v2` matches, so it proceeds.
- It detects that the init PID’s cgroup namespace inode matches the host’s PID 1 inode.
- It scans for all processes sharing the same mount namespace – including the init process and any children.
- The plugin sends SIGTERM, waits 5 seconds, then SIGKILL to each process.
- The audit log records each action.
- The container deletion completes, and no leaked process remains.

**Audit log snippet**:
```
2025-05-29T10:15:00Z [INFO] Container abc-123 (runtime io.containerd.runc.v2) init PID 9876 escaped to root cgroup
2025-05-29T10:15:00Z [INFO] Sending SIGTERM to PID 9876 (myapp)
2025-05-29T10:15:05Z [INFO] PID 9876 still running, sending SIGKILL
2025-05-29T10:15:05Z [INFO] Cleanup completed for container abc-123
```

### Example 2: Container Using a Different Runtime (e.g., io.containerd.runc.v1)

**Scenario**: A container uses an older runc runtime (not `v2`) and its init process escapes to the root cgroup.

**Outcome**:
- The plugin checks the runtime and sees it is not `io.containerd.runc.v2`.
- It returns immediately without performing any cleanup.
- The container may leak processes, but the plugin’s scope is limited to the designated runtime. Administrators can extend support by changing the plugin’s built‑in runtime list.

### Example 3: Normal Container Without Escape

**Scenario**: A regular `io.containerd.runc.v2` container runs normally, its init process stays inside its own cgroup namespace, and the container stops normally.

**Outcome**:
- The plugin obtains the init PID and reads its cgroup namespace inode.
- The inode does **not** match the host PID 1 inode.
- The plugin returns immediately, and no cleanup is performed.
- The audit log receives no entry for this container.

### Example 4: Shared Mount Namespace – Cleaning All Descendants

**Scenario**: A container spawns several child processes (e.g., a shell script that forks many times). The init process escapes to the root cgroup, but some children remain in the container’s cgroup (or also escaped). All share the same mount namespace.

**Outcome**:
- The plugin collects **every** process whose mount namespace inode equals that of the init process.
- This includes all children, grandchildren, and even processes that were reparented to the host init after the container init exited.
- Each process receives SIGTERM then SIGKILL.
- The entire process tree rooted at the container’s mount namespace is terminated.

### Example 5: Log Monitoring and Alerting

**Scenario**: An SRE team wants to detect when containers are leaking processes to the root cgroup.

**Outcome**:
- The plugin writes every escape event to the configured `log_path` (e.g., `/var/log/escape-guard/audit.log`).
- A log shipper (Fluentd, Logstash) forwards these logs to a central system (Elasticsearch, Splunk).
- The team creates an alert for any log line containing `escaped to root cgroup`.
- When the alert fires, they investigate the container image or runtime behaviour that caused the escape.

### Example 6: No Configuration Changes Required for Different Workloads

**Scenario**: The cluster runs a mix of trusted and untrusted workloads. The administrator wants a “set and forget” solution.

**Outcome**:
- The plugin works with the single `log_path` setting; no per‑container or per‑pod tuning is needed.
- It automatically handles all `io.containerd.runc.v2` containers, regardless of image or security context.
- The 5‑second SIGTERM grace period is short enough not to delay container deletion significantly, but long enough to allow well‑behaved processes to flush logs.
- Hardcoded timeout (30 seconds) ensures containerd is never blocked indefinitely.
