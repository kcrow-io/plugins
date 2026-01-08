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

// ApplyIOLimit applies io bandwidth limit to a container's cgroup
func ApplyIOLimit(cgroupPath string, device *DeviceNumber, bpsLimit uint64) error {
	if cgroups.IsCgroup2UnifiedMode() {
		return applyIOLimitV2(cgroupPath, device, bpsLimit)
	}
	return applyIOLimitV1(cgroupPath, device, bpsLimit)
}

// applyIOLimitV2 applies io limit for cgroup v2
func applyIOLimitV2(cgroupPath string, device *DeviceNumber, bpsLimit uint64) error {
	// Format: "major:minor wbps=limit"
	limit := fmt.Sprintf("%s wbps=%d", device.String(), bpsLimit)

	// Write using the unified cgroup file writer
	if err := cgroup.WriteCgroupFile(cgroupPath, "", "io.max", limit); err != nil {
		return fmt.Errorf("failed to write io.max: %w", err)
	}
	return nil
}

// applyIOLimitV1 applies io limit for cgroup v1
func applyIOLimitV1(cgroupPath string, device *DeviceNumber, bpsLimit uint64) error {
	// Format: "major:minor limit"
	limit := fmt.Sprintf("%s %d", device.String(), bpsLimit)

	// Write using the unified cgroup file writer
	if err := cgroup.WriteCgroupFile(cgroupPath, "blkio", "blkio.throttle.write_bps_device", limit); err != nil {
		return fmt.Errorf("failed to write write bps limit: %w", err)
	}
	return nil
}
