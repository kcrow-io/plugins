# Override Plugin

The override plugin is an NRI plugin that allows you to override container configurations according to OCI specification config files. It can modify container settings like environment variables, resource limits (rlimits), OOM score adjustments, and hooks.

## Features

- **Environment Variables**: Override or add environment variables to containers
- **Resource Limits (rlimits)**: Set POSIX resource limits for containers
- **OOM Score Adjustment**: Modify the Out-of-Memory killer score adjustment
- **Container Hooks**: Add lifecycle hooks (prestart, poststart, poststop)
- **OCI Spec Compatibility**: Uses standard OCI runtime specification format

## How it Works

The plugin reads an OCI specification configuration and applies the specified overrides to containers during creation. It supports the following OCI spec sections:

- `process.env` - Environment variables
- `process.rlimits` - Resource limits
- `process.oomScoreAdj` - OOM score adjustment
- `hooks` - Container lifecycle hooks

## Configuration

The plugin accepts configuration in OCI runtime specification format. You can provide configuration through the NRI plugin configuration system.

### Supported Resource Limits

The plugin supports the following POSIX resource limits:

- `RLIMIT_CPU` - CPU time limit
- `RLIMIT_FSIZE` - File size limit
- `RLIMIT_DATA` - Data segment size limit
- `RLIMIT_STACK` - Stack size limit
- `RLIMIT_CORE` - Core file size limit
- `RLIMIT_RSS` - Resident set size limit
- `RLIMIT_NPROC` - Number of processes limit
- `RLIMIT_NOFILE` - Number of open files limit
- `RLIMIT_MEMLOCK` - Locked memory limit
- `RLIMIT_AS` - Address space limit
- `RLIMIT_LOCKS` - File locks limit
- `RLIMIT_SIGPENDING` - Pending signals limit
- `RLIMIT_MSGQUEUE` - Message queue limit
- `RLIMIT_NICE` - Nice value limit
- `RLIMIT_RTPRIO` - Real-time priority limit
- `RLIMIT_RTTIME` - Real-time CPU time limit

### Example Configuration

```json
{
  "version": "1.0.0",
  "process": {
    "env": [
      "DEBUG=true",
      "LOG_LEVEL=info"
    ],
    "rlimits": [
      {
        "type": "RLIMIT_NOFILE",
        "hard": 65536,
        "soft": 65536
      },
      {
        "type": "RLIMIT_NPROC",
        "hard": 4096,
        "soft": 4096
      }
    ],
    "oomScoreAdj": -500
  },
  "hooks": {
    "prestart": [
      {
        "path": "/usr/local/bin/setup-container",
        "args": ["setup-container", "prestart"]
      }
    ]
  }
}
```

## Installation

1. Build the plugin:
   ```bash
   make build
   ```

2. Copy the binary to the NRI plugins directory:
   ```bash
   sudo cp bin/linux/amd64/override /opt/nri/bin/01-override
   ```

3. Configure the plugin through your NRI configuration system

4. Restart containerd:
   ```bash
   sudo systemctl restart containerd
   ```

## Use Cases

- **Development Environments**: Add debug environment variables and increase file limits
- **Production Hardening**: Set strict resource limits and OOM score adjustments
- **Monitoring Integration**: Add hooks for monitoring and logging systems
- **Security Policies**: Enforce consistent security settings across containers
- **Resource Management**: Apply organization-wide resource limit policies

## Notes

- The plugin applies overrides to all containers unless filtered by other mechanisms
- Configuration follows the OCI runtime specification format
- Invalid or unsupported rlimit types will be logged and skipped
- The plugin operates during container creation phase