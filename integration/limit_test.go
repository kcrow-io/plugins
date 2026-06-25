//go:build e2e

package integration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kcrow-io/plugins/integration/testenv"
	"github.com/kcrow-io/plugins/pkg/plugins/limit"
)

var limitname = "limit"

func TestLimitPlugin(t *testing.T) {
	requireCommands(t, "crictl")

	containerdPath := findContainerdPath(t)
	binPath := binaries[limitname]

	// Define I/O test scenarios with struct configs
	ioScenarios := []struct {
		name        string
		config      limit.Config
		expectedBps uint64
	}{
		{
			name: "default_io_config",
			config: limit.Config{
				WatchInterval: 5,
				Io: &limit.IoLimit{
					MaxDiskBytes: 10485760,
					BpsLimit:     1048576,
					IopsLimit:    5,
				},
				Memory: &limit.MemLimit{
					PodsUsagePercent: 20,
					CacheRssRatio:    3.5,
					MinCacheBytes:    10485760,
				},
			},
			expectedBps: 1048576,
		},
		{
			name: "strict_io_limits",
			config: limit.Config{
				WatchInterval: 5,
				Io: &limit.IoLimit{
					MaxDiskBytes: 5242880,
					BpsLimit:     524288,
					IopsLimit:    2,
				},
				Memory: &limit.MemLimit{
					PodsUsagePercent: 20,
					CacheRssRatio:    3.5,
					MinCacheBytes:    10485760,
				},
			},
			expectedBps: 524288,
		},
		{
			name: "disabled_io",
			config: limit.Config{
				WatchInterval: 5,
				Io: &limit.IoLimit{
					MaxDiskBytes: 0,
					BpsLimit:     0,
					IopsLimit:    0,
				},
				Memory: &limit.MemLimit{
					PodsUsagePercent: 20,
					CacheRssRatio:    3.5,
					MinCacheBytes:    10485760,
				},
			},
			expectedBps: 0,
		},
	}

	// Create test environment once
	env := testenv.NewContainerdTestEnvWithPath(t, containerdPath)
	defer env.Stop()

	// Set log path for all scenarios
	logPath := filepath.Join(env.LogDir(), "limit.log")

	// Install limit plugin once
	initialConfig := limit.Config{
		ContainerdConfigPath: env.ConfigPath(),
		WatchInterval:        5,
		Io: &limit.IoLimit{
			MaxDiskBytes: 10485760,
			BpsLimit:     1048576,
			IopsLimit:    5,
		},
		Memory: &limit.MemLimit{
			PodsUsagePercent: 20,
			CacheRssRatio:    3.5,
			MinCacheBytes:    10485760,
		},
		LogPath: logPath,
	}
	initialConfigJSON, _ := json.Marshal(initialConfig)
	if err := env.InstallPlugin(7, limitname, binPath, string(initialConfigJSON)); err != nil {
		t.Fatalf("failed to install limit plugin: %v", err)
	}

	// Start containerd
	if err := env.Setup(); err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	for _, scenario := range ioScenarios {
		t.Run("IO/"+scenario.name, func(t *testing.T) {
			// Set log path and containerd config path
			scenario.config.LogPath = logPath
			scenario.config.ContainerdConfigPath = env.ConfigPath()

			// Clear log file
			if err := env.ClearPluginLog(limitname); err != nil {
				t.Fatalf("failed to clear plugin log: %v", err)
			}

			// Update config
			configJSON, _ := json.Marshal(scenario.config)
			if err := env.UpdatePluginConfig(limitname, string(configJSON)); err != nil {
				t.Fatalf("failed to update plugin config: %v", err)
			}

			// Restart containerd to apply new config
			if err := env.Restart(); err != nil {
				t.Fatalf("failed to restart containerd: %v", err)
			}

			// Wait for plugin to register
			time.Sleep(2 * time.Second)

			ctx := context.Background()

			// Create sandbox with host network
			sandboxID, err := env.CreateSandbox(ctx, "io-test-"+scenario.name, &testenv.SandboxOption{
				HostNetwork: true,
			})
			if err != nil {
				t.Fatalf("failed to create sandbox: %v", err)
			}
			defer func() {
				env.StopSandbox(context.Background(), sandboxID)
				env.RemoveSandbox(context.Background(), sandboxID)
			}()

			// Create container that will generate I/O
			containerID, err := env.CreateContainer(ctx, sandboxID, testenv.CreateContainerOption{
				Name:    "io-container-" + scenario.name,
				Command: []string{"sh", "-c", "dd if=/dev/zero of=/tmp/testfile bs=1M count=20 && sync && sleep 60"},
			})
			if err != nil {
				t.Fatalf("failed to create container: %v", err)
			}
			defer func() {
				env.StopContainer(context.Background(), containerID)
				env.RemoveContainer(context.Background(), containerID)
			}()

			// Start container
			if err := env.StartContainer(ctx, containerID); err != nil {
				t.Fatalf("failed to start container: %v", err)
			}

			// Wait for I/O and plugin processing
			time.Sleep(10 * time.Second)

			// Verify cgroup configuration
			if err := env.VerifyContainerCgroup(containerID,
				testenv.ExpectCgroupExists(),
				testenv.ExpectIOLimit(scenario.expectedBps),
			); err != nil {
				env.PrintDebugInfo(containerID, limitname)
				t.Fatalf("cgroup verification failed: %v", err)
			}

			t.Logf("I/O scenario passed: cgroup version=%v", env.CgroupVersion())
		})
	}
}
