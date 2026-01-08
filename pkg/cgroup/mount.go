package cgroup

import (
	"fmt"
	"os"
	"strings"

	"github.com/opencontainers/cgroups"
)

// FindSubsystemMountPoint finds the mount point for a specific cgroup subsystem
// For v1: returns /sys/fs/cgroup/memory, /sys/fs/cgroup/blkio, etc.
// For v2: returns /sys/fs/cgroup
func FindSubsystemMountPoint(subsystem string) (string, error) {
	// For cgroup v2, all subsystems are under the unified hierarchy
	if cgroups.IsCgroup2UnifiedMode() {
		return "/sys/fs/cgroup", nil
	}

	// For cgroup v1, try common paths first
	commonPaths := []string{
		fmt.Sprintf("/sys/fs/cgroup/%s", subsystem),
		fmt.Sprintf("/sys/fs/cgroup/unified/%s", subsystem),
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Fallback: parse /proc/mounts to find the subsystem mount
	mounts, err := ParseMountInfo()
	if err != nil {
		return "", fmt.Errorf("failed to parse mount info: %w", err)
	}

	if mountPoint, ok := mounts[subsystem]; ok {
		return mountPoint, nil
	}

	return "", fmt.Errorf("%s cgroup mount point not found", subsystem)
}

// ParseMountInfo parses /proc/mounts to get cgroup mount information
// Returns a map of subsystem name to mount point
func ParseMountInfo() (map[string]string, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc/mounts: %w", err)
	}

	mounts := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		// Check if this is a cgroup mount
		if fields[2] != "cgroup" {
			continue
		}

		mountPoint := fields[1]
		options := strings.Split(fields[3], ",")

		// Extract subsystem names from mount options
		for _, opt := range options {
			// Skip common options that aren't subsystem names
			if opt == "rw" || opt == "nosuid" || opt == "nodev" || opt == "noexec" || opt == "relatime" {
				continue
			}
			// This is likely a subsystem name
			mounts[opt] = mountPoint
		}
	}

	return mounts, nil
}
