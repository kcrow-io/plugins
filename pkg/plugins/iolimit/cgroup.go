package iolimit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// CgroupVersion represents the cgroup version
type CgroupVersion int

const (
	// CgroupV1 represents cgroup v1
	CgroupV1 CgroupVersion = 1
	// CgroupV2 represents cgroup v2
	CgroupV2 CgroupVersion = 2
)

// DetectCgroupVersion detects the cgroup version
func DetectCgroupVersion() CgroupVersion {
	// Check if cgroup v2 is mounted
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return CgroupV2
	}
	return CgroupV1
}

// ApplyIOLimit applies io bandwidth limit to a container's cgroup
func ApplyIOLimit(cgroupPath string, device *DeviceNumber, bpsLimit int64, version CgroupVersion) error {
	if version == CgroupV2 {
		return applyIOLimitV2(cgroupPath, device, bpsLimit)
	}
	return applyIOLimitV1(cgroupPath, device, bpsLimit)
}

// RemoveIOLimit removes io bandwidth limit from a container's cgroup
func RemoveIOLimit(cgroupPath string, device *DeviceNumber, version CgroupVersion) error {
	if version == CgroupV2 {
		return removeIOLimitV2(cgroupPath, device)
	}
	return removeIOLimitV1(cgroupPath, device)
}

// applyIOLimitV2 applies io limit for cgroup v2
func applyIOLimitV2(cgroupPath string, device *DeviceNumber, bpsLimit int64) error {
	ioMaxPath := filepath.Join("/sys/fs/cgroup", cgroupPath, "io.max")

	// Format: "major:minor wbps=limit"
	limit := fmt.Sprintf("%s wbps=%d\n", device.String(), bpsLimit)

	if err := os.WriteFile(ioMaxPath, []byte(limit), 0644); err != nil {
		return fmt.Errorf("failed to write io.max: %w", err)
	}

	logrus.Infof("Applied io limit (v2) to %s: %s", cgroupPath, strings.TrimSpace(limit))
	return nil
}

// removeIOLimitV2 removes io limit for cgroup v2
func removeIOLimitV2(cgroupPath string, device *DeviceNumber) error {
	ioMaxPath := filepath.Join("/sys/fs/cgroup", cgroupPath, "io.max")

	// Format: "major:minor wbps=max"
	limit := fmt.Sprintf("%s wbps=max\n", device.String())

	if err := os.WriteFile(ioMaxPath, []byte(limit), 0644); err != nil {
		return fmt.Errorf("failed to write io.max: %w", err)
	}

	logrus.Infof("Removed io limit (v2) from %s", cgroupPath)
	return nil
}

// applyIOLimitV1 applies io limit for cgroup v1
func applyIOLimitV1(cgroupPath string, device *DeviceNumber, bpsLimit int64) error {
	blkioPath := filepath.Join("/sys/fs/cgroup/blkio", cgroupPath)

	// Format: "major:minor limit"
	limit := fmt.Sprintf("%s %d\n", device.String(), bpsLimit)

	// Apply write limit
	writePath := filepath.Join(blkioPath, "blkio.throttle.write_bps_device")
	if err := os.WriteFile(writePath, []byte(limit), 0644); err != nil {
		return fmt.Errorf("failed to write write bps limit: %w", err)
	}

	logrus.Infof("Applied io limit (v1) to %s: %s", cgroupPath, strings.TrimSpace(limit))
	return nil
}

// removeIOLimitV1 removes io limit for cgroup v1
func removeIOLimitV1(cgroupPath string, device *DeviceNumber) error {
	blkioPath := filepath.Join("/sys/fs/cgroup/blkio", cgroupPath)

	// Format: "major:minor 0" to remove limit
	limit := fmt.Sprintf("%s 0\n", device.String())

	// Remove write limit
	writePath := filepath.Join(blkioPath, "blkio.throttle.write_bps_device")
	if err := os.WriteFile(writePath, []byte(limit), 0644); err != nil {
		return fmt.Errorf("failed to remove write bps limit: %w", err)
	}

	logrus.Infof("Removed io limit (v1) from %s", cgroupPath)
	return nil
}
