package iolimit

import (
	"context"
	"fmt"

	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/nri/pkg/stub"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/kcrow-io/plugins/pkg/plugins"
	"github.com/sirupsen/logrus"
)

var _ plugins.Pluginer = (*ioPlugin)(nil)

const (
	name = "iolimit"
)

type ioPlugin struct {
	log     *logrus.Entry
	config  *Config
	watcher *Watcher
}

// New creates a new io plugin
func New() (*ioPlugin, error) {
	return &ioPlugin{
		log:    log.G(context.Background()).WithField("plugin", name),
		config: NewConfig(),
	}, nil
}

// Name returns the plugin name
func (p *ioPlugin) Name() string {
	return name
}

// Configure is called by NRI to configure the plugin
func (p *ioPlugin) Configure(ctx context.Context, config, runtime, version string) (stub.EventMask, error) {
	p.log.Infof("Configuring plugin (runtime: %s, version: %s)", runtime, version)

	err := p.config.Parse([]byte(config))
	if err != nil {
		return 0, fmt.Errorf("failed to parse config: %w", err)
	}

	p.log.Infof("Containerd config: socket=%s, root=%s",
		p.config.ContainerdSocket, p.config.ContainerdRoot)

	// Detect device number using containerd root
	device, err := GetDeviceNumberFromPath(p.config.ContainerdRoot)
	if err != nil {
		return 0, fmt.Errorf("failed to get device number: %w", err)
	}

	p.log.Infof("Detected device number: %s", device.String())

	// Create containerd client
	client, err := client.New(p.config.ContainerdSocket, client.WithDefaultNamespace(DefaultNamespace))
	if err != nil {
		return 0, fmt.Errorf("failed to create containerd client: %w", err)
	}

	// Create and start watcher
	p.watcher = NewWatcher(client, p.config, device, p.config.ContainerdRoot, "overlayfs")
	p.watcher.Start()

	p.log.Info("Plugin configured successfully")

	// We don't need to subscribe to any NRI events since we're using a background watcher
	return 0, nil
}
