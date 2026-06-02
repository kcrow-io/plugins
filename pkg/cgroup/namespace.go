package cgroup

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// GetNsInode returns the inode number of a namespace file
func GetNsInode(path string) (uint64, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return 0, fmt.Errorf("failed to stat %s: %w", path, err)
	}
	return stat.Ino, nil
}

// GetCgroupNsInode returns the cgroup namespace inode of a process
func GetCgroupNsInode(pid int) (uint64, error) {
	path := fmt.Sprintf("/proc/%d/ns/cgroup", pid)
	return GetNsInode(path)
}

// GetMountNsInode returns the mount namespace inode of a process
func GetMountNsInode(pid int) (uint64, error) {
	path := fmt.Sprintf("/proc/%d/ns/mnt", pid)
	return GetNsInode(path)
}

// FindProcessesWithMountNs finds all processes sharing the same mount namespace
func FindProcessesWithMountNs(targetInode uint64) ([]int, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("failed to read /proc: %w", err)
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		inode, err := GetMountNsInode(pid)
		if err != nil {
			continue
		}

		if inode == targetInode {
			pids = append(pids, pid)
		}
	}

	return pids, nil
}

// IsEscapedToRootCgroup checks if a process has escaped to the root cgroup namespace
// by comparing its cgroup namespace inode with the host PID 1's cgroup namespace inode
func IsEscapedToRootCgroup(pid int, hostCgroupNsInode uint64) (bool, error) {
	pidInode, err := GetCgroupNsInode(pid)
	if err != nil {
		return false, fmt.Errorf("failed to get cgroup namespace inode for PID %d: %w", pid, err)
	}

	return pidInode == hostCgroupNsInode, nil
}

// GetHostCgroupNsInode returns the cgroup namespace inode of the host PID 1
func GetHostCgroupNsInode() (uint64, error) {
	return GetCgroupNsInode(1)
}

// GetProcessName returns the process name by reading /proc/[pid]/comm
func GetProcessName(pid int) string {
	commPath := fmt.Sprintf("/proc/%d/comm", pid)
	data, err := os.ReadFile(commPath)
	if err != nil {
		return "unknown"
	}
	return string(data[:len(data)-1]) // Remove trailing newline
}

// IsInRootCgroup checks if a process is in the root cgroup by reading /proc/{pid}/cgroup
// For cgroup v2, the format is: "0::<path>"
// If path is "/", the process is in the root cgroup
func IsInRootCgroup(pid int) (bool, error) {
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	file, err := os.Open(cgroupPath)
	if err != nil {
		return false, fmt.Errorf("failed to open %s: %w", cgroupPath, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// cgroup v2 format: "0::<path>"
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			return strings.TrimSpace(path) == "/", nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("failed to read %s: %w", cgroupPath, err)
	}

	return false, nil
}

// GetCgroupPath returns the cgroup path of a process by reading /proc/{pid}/cgroup
// For cgroup v2, returns the path from "0::<path>"
func GetCgroupPath(pid int) (string, error) {
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	file, err := os.Open(cgroupPath)
	if err != nil {
		return "", fmt.Errorf("failed to open %s: %w", cgroupPath, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// cgroup v2 format: "0::<path>"
		if path, ok := strings.CutPrefix(line, "0::"); ok {
			return strings.TrimSpace(path), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read %s: %w", cgroupPath, err)
	}

	return "", fmt.Errorf("no cgroup v2 entry found for pid %d", pid)
}

// IsEscapedFromCgroup checks if a process has escaped from its expected cgroup
// expectedCgroupPath is the container's cgroup path (e.g., from Linux.CgroupsPath)
// Returns true if the process's actual cgroup path does NOT match the expected path
func IsEscapedFromCgroup(pid int, expectedCgroupPath string) (bool, error) {
	actualPath, err := GetCgroupPath(pid)
	if err != nil {
		return false, err
	}

	// If expected path is empty, we can't determine escape
	if expectedCgroupPath == "" {
		return false, nil
	}

	// If actual path is "/" (root), it's definitely escaped
	if actualPath == "/" {
		return true, nil
	}

	// Convert expected path to filesystem format
	expectedFsPath := ConvertSystemdPathToFs(expectedCgroupPath)

	// Normalize both paths for comparison
	actual := strings.TrimPrefix(actualPath, "/")
	expected := strings.TrimPrefix(expectedFsPath, "/")

	// Check if actual path matches expected path
	return !strings.Contains(actual, expected), nil
}
