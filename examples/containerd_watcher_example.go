package main

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/containerd/v2/client"
	"github.com/kcrow-io/plugins/pkg/containerd"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Example demonstrates how to use the new subscribe pattern with the watcher

func main() {
	// Parse containerd config
	cntrd, err := containerd.ParseContainerdConfig(containerd.DefaultContainerdConfigPath)
	if err != nil {
		panic(err)
	}
	defer cntrd.Close() // nolint

	// Create watcher config with CRI socket
	config := &containerd.Config{
		WatchInterval: 5 * time.Second,
		CRISocket:     "unix:///run/containerd/containerd.sock",
	}

	// Create watcher
	watcher, err := containerd.NewWatcher(cntrd, config)
	if err != nil {
		panic(err)
	}

	// Example 1: Subscribe to containerd containers
	// This subscriber will process all containers
	watcher.Subscribe("logger", func(ctx context.Context, c client.Container) bool {
		info, err := c.Info(ctx)
		if err != nil {
			fmt.Printf("Failed to get container info: %v\n", err)
			return true // continue even on error
		}
		fmt.Printf("Containerd Container: %s (labels: %v)\n", info.ID, info.Labels)
		return true // continue to next container
	})

	// Example 2: Subscribe with early exit
	// This subscriber will stop after finding a specific container
	watcher.Subscribe("finder", func(ctx context.Context, c client.Container) bool {
		info, err := c.Info(ctx)
		if err != nil {
			return true
		}

		// Stop this subscriber if we find a container with specific label
		if info.Labels["app"] == "nginx" {
			fmt.Printf("Found nginx container: %s\n", info.ID)
			return false // stop this subscriber
		}
		return true // continue
	})

	// Example 3: Subscribe to CRI containers
	// This subscriber uses the CRI API to get container information
	watcher.SubscribeCRI("cri-logger", func(ctx context.Context, c *runtimeapi.Container) bool {
		fmt.Printf("CRI Container: %s (state: %s, image: %s)\n",
			c.Id, c.State.String(), c.Image.Image)
		return true // continue
	})

	// Example 4: CRI subscriber with filtering
	// Only process running containers
	watcher.SubscribeCRI("running-only", func(ctx context.Context, c *runtimeapi.Container) bool {
		if c.State == runtimeapi.ContainerState_CONTAINER_RUNNING {
			fmt.Printf("Running container: %s\n", c.Id)
		}
		return true // continue
	})

	// Example 5: CRI subscriber that stops after finding a match
	watcher.SubscribeCRI("cri-finder", func(ctx context.Context, c *runtimeapi.Container) bool {
		// Stop after finding a container from a specific pod
		if c.PodSandboxId == "target-pod-id" {
			fmt.Printf("Found target pod container: %s\n", c.Id)
			return false // stop this subscriber
		}
		return true // continue
	})

	// Start the watcher
	watcher.Start()

	// Run for 30 seconds
	time.Sleep(30 * time.Second)

	// Stop the watcher
	watcher.Stop()

	// You can also unsubscribe specific subscribers
	// watcher.Unsubscribe("logger")
}
