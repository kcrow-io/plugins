package cgroup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/opencontainers/cgroups"
)

// WriteCgroupFile writes to a cgroup control file (low-level generic function)
// cgroupPath: cgroup path (will automatically handle systemd conversion)
// subsystem: subsystem name, used for v1 (e.g., "memory", "blkio"), empty string for v2
// filename: file name (e.g., "memory.high", "io.max")
// value: value to write
// version: cgroup version
func WriteCgroupFile(cgroupPath, subsystem, filename, value string) error {
	// Get the full file path
	filePath, err := GetCgroupFilePath(cgroupPath, subsystem, filename)
	if err != nil {
		return err
	}

	// Validate filePath
	if _, err := os.Stat(filePath); err != nil {
		return err
	}

	// Write the file
	if err := os.WriteFile(filePath, []byte(value), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filePath, err)
	}

	return nil
}

// ReadCgroupFile reads from a cgroup control file (low-level generic function)
// cgroupPath: cgroup path (will automatically handle systemd conversion)
// subsystem: subsystem name, used for v1 (e.g., "memory", "blkio"), empty string for v2
// filename: file name (e.g., "memory.high", "io.max")
// version: cgroup version
func ReadCgroupFile(cgroupPath, subsystem, filename string) (string, error) {
	// Get the full file path
	filePath, err := GetCgroupFilePath(cgroupPath, subsystem, filename)
	if err != nil {
		return "", err
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filePath, err)
	}

	return string(data), nil
}

// GetCgroupFilePath gets the complete filesystem path for a cgroup file
// Returns: /sys/fs/cgroup/... or /sys/fs/cgroup/memory/... etc.
func GetCgroupFilePath(cgroupPath, subsystem, filename string) (string, error) {
	// Normalize the cgroup path (handle systemd conversion)
	normalizedPath := NormalizeCgroupPath(cgroupPath)

	if cgroups.IsCgroup2UnifiedMode() {
		// For v2: /sys/fs/cgroup/{cgroupPath}/{filename}
		return filepath.Join("/sys/fs/cgroup", normalizedPath, filename), nil
	}

	// For v1: need to find the subsystem mount point
	if subsystem == "" {
		return "", fmt.Errorf("subsystem is required for cgroup v1")
	}

	mountPoint, err := FindSubsystemMountPoint(subsystem)
	if err != nil {
		return "", err
	}

	// For v1: {mountPoint}/{cgroupPath}/{filename}
	return filepath.Join(mountPoint, normalizedPath, filename), nil
}

// ValidateCgroupPath validates that a cgroup path exists
func ValidateCgroupPath(cgroupPath string) error {
	// Normalize the cgroup path
	normalizedPath := NormalizeCgroupPath(cgroupPath)

	var dirPath string
	if cgroups.IsCgroup2UnifiedMode() {
		dirPath = filepath.Join("/sys/fs/cgroup", normalizedPath)
	} else {
		// For v1, we can't validate without knowing the subsystem
		// Just check if the base cgroup directory exists
		dirPath = "/sys/fs/cgroup"
	}

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return fmt.Errorf("cgroup directory does not exist: %s", dirPath)
	}

	return nil
}
