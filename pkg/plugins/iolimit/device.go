package iolimit

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DeviceNumber represents a device major:minor number
type DeviceNumber struct {
	Major uint64
	Minor uint64
}

// String returns the device number in "major:minor" format
func (d DeviceNumber) String() string {
	return fmt.Sprintf("%d:%d", d.Major, d.Minor)
}

// GetSnapshotDeviceNumber gets the device number for the snapshotter directory
func GetSnapshotDeviceNumber(root, snapshotter string) (*DeviceNumber, error) {
	if snapshotter != "overlayfs" {
		return nil, fmt.Errorf("only overlayfs snapshotter is supported, got: %s", snapshotter)
	}

	// Construct the snapshots directory path
	snapshotsPath := filepath.Join(root, "io.containerd.snapshotter.v1.overlayfs", "snapshots")

	// Get file info
	var stat syscall.Stat_t
	if err := syscall.Stat(snapshotsPath, &stat); err != nil {
		// If snapshots directory doesn't exist, try the parent directory
		parentPath := filepath.Join(root, "io.containerd.snapshotter.v1.overlayfs")
		if err := syscall.Stat(parentPath, &stat); err != nil {
			// If parent doesn't exist either, use root directory
			if err := syscall.Stat(root, &stat); err != nil {
				return nil, fmt.Errorf("failed to stat directory %s: %w", root, err)
			}
		}
	}

	// Extract major and minor device numbers
	dev := stat.Dev
	major := (dev >> 8) & 0xff
	minor := dev & 0xff

	return &DeviceNumber{
		Major: major,
		Minor: minor,
	}, nil
}

// GetDeviceNumberFromPath gets the device number for any path
func GetDeviceNumberFromPath(path string) (*DeviceNumber, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path does not exist: %s", path)
		}
		return nil, fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	dev := stat.Dev
	major := (dev >> 8) & 0xff
	minor := dev & 0xff

	return &DeviceNumber{
		Major: major,
		Minor: minor,
	}, nil
}
