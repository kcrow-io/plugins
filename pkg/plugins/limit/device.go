package limit

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// DeviceNumber represents a device major:minor number
type DeviceNumber struct {
	Major uint32
	Minor uint32
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

	return &DeviceNumber{
		Major: unix.Major(dev),
		Minor: unix.Minor(dev),
	}, nil
}

func GetPathUsage(path string) (usage uint64, err error) {
	var stat syscall.Statfs_t

	err = syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)

	if total > 0 {
		return uint64(float64(total-free) / float64(total) * 100), nil
	}

	return 0, fmt.Errorf("total is zero")
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

	return &DeviceNumber{
		Major: unix.Major(dev),
		Minor: unix.Minor(dev),
	}, nil
}
