//go:build e2e

package integration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kcrow-io/plugins/integration/testenv"
	"github.com/kcrow-io/plugins/pkg/plugins/memory"
)

var memname = "memory"

func TestMemoryPlugin(t *testing.T) {
	requireCommands(t, "crictl")

	containerdPath := findContainerdPath(t)
	binPath := binaries[memname]

	// Define test scenarios with struct configs
	scenarios := []struct {
		name        string
		config      memory.Config
		memoryLimit int64
	}{
		{
			name: "small_limit",
			config: memory.Config{
				Disabled:  false,
				LogPath:   "", // Will be set later
				HighRatio: 0.9,
			},
			memoryLimit: 64 * 1024 * 1024, // 64MB
		},
		{
			name: "disabled_plugin",
			config: memory.Config{
				Disabled:  true,
				LogPath:   "",
				HighRatio: 0.8,
			},
			memoryLimit: 256 * 1024 * 1024,
		},
		{
			name: "no_memory_limit",
			config: memory.Config{
				Disabled:  false,
				LogPath:   "",
				HighRatio: 0.8,
			},
			memoryLimit: 0, // No limit
		},
	}

	// Create test environment once
	env := testenv.NewContainerdTestEnvWithPath(t, containerdPath)
	defer env.Stop()

	// Set log path for all scenarios
	logPath := filepath.Join(env.LogDir(), "memory.log")

	// Install memory plugin once
	initialConfig := memory.Config{
		Disabled:  false,
		LogPath:   logPath,
		HighRatio: 0.8,
	}
	initialConfigJSON, _ := json.Marshal(initialConfig)
	if err := env.InstallPlugin(6, memname, binPath, string(initialConfigJSON)); err != nil {
		t.Fatalf("failed to install memory plugin: %v", err)
	}

	// Start containerd
	if err := env.Setup(); err != nil {
		t.Fatalf("failed to setup test environment: %v", err)
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Set log path for this scenario
			scenario.config.LogPath = logPath

			// Clear log file
			if err := env.ClearPluginLog(memname); err != nil {
				t.Fatalf("failed to clear plugin log: %v", err)
			}

			// Update config
			configJSON, _ := json.Marshal(scenario.config)
			if err := env.UpdatePluginConfig(memname, string(configJSON)); err != nil {
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
			sandboxID, err := env.CreateSandbox(ctx, "memory-test-"+scenario.name, &testenv.SandboxOption{
				HostNetwork: true,
			})
			if err != nil {
				t.Fatalf("failed to create sandbox: %v", err)
			}
			defer func() {
				env.StopSandbox(context.Background(), sandboxID)
				env.RemoveSandbox(context.Background(), sandboxID)
			}()

			// Create container with memory limit
			containerID, err := env.CreateContainer(ctx, sandboxID, testenv.CreateContainerOption{
				Name:        "memory-container-" + scenario.name,
				MemoryLimit: scenario.memoryLimit,
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

			// Wait for plugin to process
			time.Sleep(2 * time.Second)

			// Verify cgroup configuration for non-disabled scenarios
			if !scenario.config.Disabled && scenario.memoryLimit > 0 {
				if err := env.VerifyContainerCgroup(containerID,
					testenv.ExpectCgroupExists(),
					testenv.ExpectMemoryHigh(scenario.config.HighRatio, scenario.memoryLimit),
				); err != nil {
					env.PrintDebugInfo(containerID, memname)
					t.Fatalf("cgroup verification failed: %v", err)
				}
			}

			t.Logf("Scenario passed: cgroup version=%v", env.CgroupVersion())
		})
	}
}
