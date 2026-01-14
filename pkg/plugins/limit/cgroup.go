package limit

import (
	"fmt"

	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/opencontainers/cgroups"
)

// ApplyIOLimit applies io bandwidth limit to a container's cgroup
func ApplyCleanCache(cgroupPath string, size uint64) error {
	if cgroups.IsCgroup2UnifiedMode() {
		return applyCleanV2(cgroupPath, size)
	}
	return applyCleanV1(cgroupPath, size)
}

// applyCleanV2 applies io clear for cgroup v2
func applyCleanV2(cgroupPath string, size uint64) error {
	limit := fmt.Sprintf("%d", size)
	// Write using the unified cgroup file writer
	if err := cgroup.WriteCgroupFile(cgroupPath, "", "memory.reclaim", limit); err != nil {
		return fmt.Errorf("failed to write memory.reclaim: %w", err)
	}
	return nil
}

// applyCleanV1 applies io limit for cgroup v1
func applyCleanV1(cgroupPath string, _size uint64) error {
	limit := "1"
	// Write using the unified cgroup file writer
	if err := cgroup.WriteCgroupFile(cgroupPath, "memory", "memory.force_empty", limit); err != nil {
		return fmt.Errorf("failed to write memory.force_empty: %w", err)
	}
	return nil
}

// ApplyIOLimit applies or removes io bandwidth and IOPS limits to/from a container's cgroup
// If ioLimit is nil, removes all limits. Otherwise, applies the specified limits.
func ApplyIOLimit(cgroupPath string, device *DeviceNumber, ioLimit *iolimit) error {
	if cgroups.IsCgroup2UnifiedMode() {
		return applyIOLimitV2(cgroupPath, device, ioLimit)
	}
	return applyIOLimitV1(cgroupPath, device, ioLimit)
}

// applyIOLimitV2 applies or removes io limit for cgroup v2
func applyIOLimitV2(cgroupPath string, device *DeviceNumber, ioLimit *iolimit) error {
	var limitStr string

	if ioLimit == nil {
		// Remove limits by setting to "max"
		limitStr = fmt.Sprintf("%s wbps=max wiops=max", device.String())
	} else {
		// Apply limits
		limitStr = device.String()
		if ioLimit.BpsLimit > 0 {
			limitStr += fmt.Sprintf(" wbps=%d", ioLimit.BpsLimit)
		}
		if ioLimit.IopsLimit > 0 {
			limitStr += fmt.Sprintf(" wiops=%d", ioLimit.IopsLimit)
		}
	}

	// Write using the unified cgroup file writer
	if err := cgroup.WriteCgroupFile(cgroupPath, "", "io.max", limitStr); err != nil {
		return fmt.Errorf("failed to write io.max: %w", err)
	}
	return nil
}

// applyIOLimitV1 applies or removes io limit for cgroup v1
func applyIOLimitV1(cgroupPath string, device *DeviceNumber, ioLimit *iolimit) error {
	if ioLimit == nil {
		// Remove limits by setting to 0
		zeroLimit := fmt.Sprintf("%s 0", device.String())

		// Remove BPS limit
		if err := cgroup.WriteCgroupFile(cgroupPath, "blkio", "blkio.throttle.write_bps_device", zeroLimit); err != nil {
			return fmt.Errorf("failed to remove write bps limit: %w", err)
		}

		// Remove IOPS limit
		if err := cgroup.WriteCgroupFile(cgroupPath, "blkio", "blkio.throttle.write_iops_device", zeroLimit); err != nil {
			return fmt.Errorf("failed to remove write iops limit: %w", err)
		}
	} else {
		// Apply limits

		// Apply BPS limit
		if ioLimit.BpsLimit > 0 {
			bpsLimit := fmt.Sprintf("%s %d", device.String(), ioLimit.BpsLimit)
			if err := cgroup.WriteCgroupFile(cgroupPath, "blkio", "blkio.throttle.write_bps_device", bpsLimit); err != nil {
				return fmt.Errorf("failed to write write bps limit: %w", err)
			}
		}

		// Apply IOPS limit
		if ioLimit.IopsLimit > 0 {
			iopsLimit := fmt.Sprintf("%s %d", device.String(), ioLimit.IopsLimit)
			if err := cgroup.WriteCgroupFile(cgroupPath, "blkio", "blkio.throttle.write_iops_device", iopsLimit); err != nil {
				return fmt.Errorf("failed to write write iops limit: %w", err)
			}
		}
	}

	return nil
}
