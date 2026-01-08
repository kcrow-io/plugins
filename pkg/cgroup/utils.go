package cgroup

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/opencontainers/cgroups"
	cgroupsystemd "github.com/opencontainers/cgroups/systemd"
)

// CgroupVersion represents the cgroup version
type CgroupVersion int

const (
	// CgroupV1 represents cgroup v1
	CgroupV1 CgroupVersion = 1
	// CgroupV2 represents cgroup v2
	CgroupV2 CgroupVersion = 2
)

var (
	// cachedVersion caches the detected cgroup version
	cachedVersion     CgroupVersion
	versionDetectOnce sync.Once
)

// DetectCgroupVersion detects the system's cgroup version (with caching)
// This function is safe for concurrent use and will only perform detection once
func DetectCgroupVersion() CgroupVersion {
	versionDetectOnce.Do(func() {
		// Check if cgroup v2 is mounted by looking for cgroup.controllers file
		if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
			cachedVersion = CgroupV2
		} else {
			cachedVersion = CgroupV1
		}
	})
	return cachedVersion
}

// IsSystemdDriver checks if the cgroup path indicates systemd driver
// systemd driver paths contain .slice
func IsSystemdDriver(cgroupPath string) bool {
	return strings.Contains(cgroupPath, ".slice")
}

// ConvertSystemdPathToFs converts systemd cgroup path to filesystem path
// systemd uses colons as separators, e.g.:
// kubepods-burstable-pod123.slice:cri-containerd:abc123
// should "slice:prefix:name", so needs to be converted to:
// kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/cri-containerd-abc123.scope
func ConvertSystemdPathToFs(cgroupPath string) string {
	// Remove leading slash and replace colons with slashes
	if strings.HasPrefix(cgroupPath, "/") {
		return cgroupPath
	}
	parts := strings.Split(cgroupPath, ":")
	if len(parts) != 3 {
		return cgroupPath
	}
	slice, runtime, containerID := parts[0], parts[1], parts[2]
	slicePath, err := cgroupsystemd.ExpandSlice(slice)
	if err != nil {
		return cgroupPath
	}
	scopeName := fmt.Sprintf("%s-%s.scope", runtime, containerID)

	return fmt.Sprintf("%s/%s", slicePath, scopeName)
}

// NormalizeCgroupPath normalizes a cgroup path
// Handles systemd path conversion and removes leading slashes
func NormalizeCgroupPath(cgroupPath string) string {
	// If it's a systemd path, convert it
	if IsSystemdDriver(cgroupPath) {
		return ConvertSystemdPathToFs(cgroupPath)
	}
	// Otherwise just remove leading slash
	return strings.TrimPrefix(cgroupPath, "/")
}

// GetCgroupPathFromPid reads the actual cgroup path from /proc/[pid]/cgroup
// This is the most reliable way to get the real cgroup path, especially in Kubernetes
// where additional parent slices (like kubelet.slice/kubepods-burstable.slice/) are added
func GetCgroupPathFromPid(pid uint32) (string, error) {
	cgroupFile := fmt.Sprintf("/proc/%d/cgroup", pid)
	data, err := os.ReadFile(cgroupFile)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		// Format for cgroup v2: 0::/path/to/cgroup
		// Format for cgroup v1: hierarchy-ID:controller-list:path
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}

		if cgroups.IsCgroup2UnifiedMode() {
			// For v2, we look for the unified hierarchy (hierarchy-ID = 0)
			if parts[0] == "0" && parts[1] == "" {
				// Return the path without leading slash
				return strings.TrimPrefix(parts[2], "/"), nil
			}
		} else {
			// For v1, we need to find the right controller
			// We'll return the first non-empty path we find
			// (caller should specify which controller they need)
			if parts[2] != "/" && parts[2] != "" {
				return strings.TrimPrefix(parts[2], "/"), nil
			}
		}
	}

	return "", os.ErrNotExist
}
