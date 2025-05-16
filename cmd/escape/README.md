# escape plugin

This plugin allows container's main process to escape resource limits based on annotations.

## Annotation Format

Add the following annotation to your container:
```yaml
annotations:
  io.kcrow.escape: "cpu,memory"  # Can specify one or both resources
```

## Supported Resources
- `cpu`: Escape CPU limits (cgroups cpu subsystem)
- `memory`: Escape memory limits (cgroups memory subsystem)

## Example Usage

```bash
# Create container with escape annotation
ctr run --rm --runtime io.containerd.runc.v2 \
  --annotation io.kcrow.escape=cpu,memory \
  docker.io/library/alpine:latest test

# Verify process is outside cgroup limits
cat /proc/$(pgrep -f "test")/cgroup
```

## Notes
- Only affects the main process, child processes remain limited
- Requires privileged container or appropriate capabilities
