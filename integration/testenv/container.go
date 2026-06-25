package testenv

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kcrow-io/plugins/pkg/cgroup"
	"github.com/opencontainers/cgroups"
)

// ContainerInfo holds container runtime information
type ContainerInfo struct {
	PID        int    `json:"pid"`
	CgroupPath string `json:"cgroup_path"`
}

// InspectResult represents crictl inspect output
type InspectResult struct {
	Info struct {
		PID         int `json:"pid"`
		RuntimeSpec struct {
			Linux struct {
				CgroupsPath string `json:"cgroupsPath"`
			} `json:"linux"`
		} `json:"runtimeSpec"`
	} `json:"info"`
}

// GetContainerInfo retrieves container PID and cgroup path via crictl inspect
func (e *ContainerdTestEnv) GetContainerInfo(containerID string) (*ContainerInfo, error) {
	e.t.Helper()

	stdout, stderr, err := e.Crictl(context.Background(), "inspect", containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w (stderr: %s)", err, stderr)
	}

	var result InspectResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		return nil, fmt.Errorf("failed to parse inspect result: %w", err)
	}

	if result.Info.PID == 0 {
		return nil, fmt.Errorf("container PID not found")
	}

	cgroupPath := result.Info.RuntimeSpec.Linux.CgroupsPath
	if cgroupPath == "" {
		return nil, fmt.Errorf("container cgroup path not found")
	}

	return &ContainerInfo{
		PID:        result.Info.PID,
		CgroupPath: cgroupPath,
	}, nil
}

// GetContainerPID retrieves container's main process PID
func (e *ContainerdTestEnv) GetContainerPID(containerID string) (int, error) {
	e.t.Helper()

	info, err := e.GetContainerInfo(containerID)
	if err != nil {
		return 0, err
	}
	return info.PID, nil
}

// GetContainerCgroupPath retrieves container's cgroup path using pkg/cgroup
func (e *ContainerdTestEnv) GetContainerCgroupPath(containerID string) (string, error) {
	e.t.Helper()

	info, err := e.GetContainerInfo(containerID)
	if err != nil {
		return "", err
	}

	// Use pkg/cgroup to get the real cgroup path from PID
	cgroupPath, err := cgroup.GetCgroupPathFromPid(uint32(info.PID))
	if err != nil {
		// Fallback to the path from inspect
		return cgroup.NormalizeCgroupPath(info.CgroupPath), nil
	}

	return cgroupPath, nil
}

// CgroupExpectFn is a function that validates cgroup values
// Returns error if validation fails, nil if successful
type CgroupExpectFn func(cgroupPath string) error

// VerifyContainerCgroup verifies container cgroup configuration using expect functions
func (e *ContainerdTestEnv) VerifyContainerCgroup(containerID string, expectFns ...CgroupExpectFn) error {
	e.t.Helper()

	info, err := e.GetContainerInfo(containerID)
	if err != nil {
		return fmt.Errorf("failed to get container info: %w", err)
	}

	// Check if PID exists
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", info.PID)); os.IsNotExist(err) {
		return fmt.Errorf("container process %d does not exist", info.PID)
	}

	// Get real cgroup path using pkg/cgroup
	cgroupPath, err := cgroup.GetCgroupPathFromPid(uint32(info.PID))
	if err != nil {
		// Fallback to the path from inspect
		cgroupPath = cgroup.NormalizeCgroupPath(info.CgroupPath)
	}

	// Run all expect functions
	for _, expectFn := range expectFns {
		if err := expectFn(cgroupPath); err != nil {
			return err
		}
	}

	return nil
}

// ExpectMemoryHigh returns an expect function that validates memory.high
// For cgroup v2: reads memory.high
// For cgroup v1: reads memory.soft_limit_in_bytes
func ExpectMemoryHigh(expectedRatio float64, memoryLimit int64) CgroupExpectFn {
	return func(cgroupPath string) error {
		var (
			highFile  string
			limitFile string
		)

		if cgroups.IsCgroup2UnifiedMode() {
			highFile = "memory.high"
			limitFile = "memory.max"
		} else {
			highFile = "memory.soft_limit_in_bytes"
			limitFile = "memory.limit_in_bytes"
		}

		// Read memory limit using pkg/cgroup
		limitStr, err := cgroup.ReadCgroupFile(cgroupPath, "memory", limitFile)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", limitFile, err)
		}
		limitStr = strings.TrimSpace(limitStr)

		var limit int64
		if limitStr == "max" {
			limit = memoryLimit
		} else {
			limit, err = strconv.ParseInt(limitStr, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse %s value '%s': %w", limitFile, limitStr, err)
			}
		}

		// Read memory high using pkg/cgroup
		highStr, err := cgroup.ReadCgroupFile(cgroupPath, "memory", highFile)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", highFile, err)
		}
		highStr = strings.TrimSpace(highStr)

		if highStr == "max" || highStr == "0" {
			return fmt.Errorf("memory high not set (value: %s)", highStr)
		}

		high, err := strconv.ParseInt(highStr, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse %s value '%s': %w", highFile, highStr, err)
		}

		// Calculate expected high
		expectedHigh := int64(float64(limit) * expectedRatio)

		// Allow 1% tolerance
		tolerance := int64(float64(expectedHigh) * 0.01)
		if high < expectedHigh-tolerance || high > expectedHigh+tolerance {
			return fmt.Errorf("memory high mismatch: got %d, want ~%d (ratio=%.2f, limit=%d)",
				high, expectedHigh, expectedRatio, limit)
		}

		return nil
	}
}

// ExpectIOLimit returns an expect function that validates IO limits
// For cgroup v2: reads io.max
// For cgroup v1: reads blkio.throttle.read_bps_device and blkio.throttle.write_bps_device
func ExpectIOLimit(expectedBps uint64) CgroupExpectFn {
	return func(cgroupPath string) error {
		if cgroups.IsCgroup2UnifiedMode() {
			// Read io.max using pkg/cgroup
			ioMaxStr, err := cgroup.ReadCgroupFile(cgroupPath, "", "io.max")
			if err != nil {
				return fmt.Errorf("failed to read io.max: %w", err)
			}

			// Parse io.max format: "major:minor rbps=xxx wbps=xxx riops=xxx wiops=xxx"
			lines := strings.Split(ioMaxStr, "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					for _, part := range parts {
						if strings.HasPrefix(part, "wbps=") {
							bpsStr := strings.TrimPrefix(part, "wbps=")
							if bpsStr == "max" {
								// wbps=max means no limit
								if expectedBps == 0 {
									return nil
								}
								return fmt.Errorf("IO write BPS mismatch: got max, want %d", expectedBps)
							}
							bps, err := strconv.ParseUint(bpsStr, 10, 64)
							if err != nil {
								return fmt.Errorf("failed to parse wbps: %w", err)
							}
							if bps != expectedBps {
								return fmt.Errorf("IO write BPS mismatch: got %d, want %d", bps, expectedBps)
							}
							return nil
						}
					}
				}
			}
			// If no wbps field found and expectedBps is 0, that's OK (IO disabled)
			if expectedBps == 0 {
				return nil
			}
			return fmt.Errorf("wbps not found in io.max")
		}

		// For v1, check blkio.throttle using pkg/cgroup
		writeBpsStr, err := cgroup.ReadCgroupFile(cgroupPath, "blkio", "blkio.throttle.write_bps_device")
		if err != nil {
			return fmt.Errorf("failed to read blkio.throttle.write_bps_device: %w", err)
		}

		lines := strings.Split(writeBpsStr, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				bps, err := strconv.ParseUint(parts[1], 10, 64)
				if err != nil {
					return fmt.Errorf("failed to parse BPS: %w", err)
				}
				if bps != expectedBps {
					return fmt.Errorf("IO write BPS mismatch: got %d, want %d", bps, expectedBps)
				}
				return nil
			}
		}

		return fmt.Errorf("device not found in blkio.throttle.write_bps_device")
	}
}

// ExpectCgroupExists returns an expect function that validates cgroup path exists
func ExpectCgroupExists() CgroupExpectFn {
	return func(cgroupPath string) error {
		// Use pkg/cgroup to validate
		return cgroup.ValidateCgroupPath(cgroupPath)
	}
}

// PrintDebugInfo prints debug information when test fails
func (e *ContainerdTestEnv) PrintDebugInfo(containerID, pluginName string) {
	e.t.Helper()

	e.t.Log("=== Debug Information ===")

	// Print container info
	info, err := e.GetContainerInfo(containerID)
	if err != nil {
		e.t.Logf("Failed to get container info: %v", err)
	} else {
		e.t.Logf("Container PID: %d", info.PID)
		e.t.Logf("Container CgroupPath (from inspect): %s", info.CgroupPath)

		// Get real cgroup path from PID
		cgroupPath, err := cgroup.GetCgroupPathFromPid(uint32(info.PID))
		if err != nil {
			e.t.Logf("Failed to get cgroup path from PID: %v", err)
		} else {
			e.t.Logf("Container CgroupPath (from /proc): %s", cgroupPath)
		}
	}

	// Print plugin log
	if pluginName != "" {
		logContent, err := e.ReadPluginLog(pluginName)
		if err != nil {
			e.t.Logf("Failed to read plugin log: %v", err)
		} else {
			e.t.Logf("=== Plugin Log (%s) ===", pluginName)
			e.t.Log(logContent)
		}
	}

	// Print containerd log
	containerdLogPath := e.rootDir + "/containerd/containerd.log"
	if data, err := os.ReadFile(containerdLogPath); err == nil {
		e.t.Log("=== Containerd Log ===")
		e.t.Log(string(data))
	}

	e.t.Log("=== End Debug Information ===")
}
