package containerd

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/containerd/containerd/v2/client"
)

const (
	DefaultNamespace            = "k8s.io"
	DefaultContainerdConfigPath = "/etc/containerd/config.toml"
)

// ContainerdConfig represents the relevant parts of containerd's config.toml
type containerdConfig struct {
	Root string `toml:"root"`
	GRPC struct {
		Address string `toml:"address,omit"`
	} `toml:"grpc,omit"`
}

type Cntrd struct {
	Root string
	*client.Client
}

// ParseContainerdConfig parses containerd's config.toml file
func ParseContainerdConfig(configPath string) (*Cntrd, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("containerd config file not found: %s", configPath)
	}

	var config containerdConfig
	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return nil, fmt.Errorf("failed to parse containerd config: %w", err)
	}
	if config.GRPC.Address == "" {
		config.GRPC.Address = "/run/containerd/containerd.sock"
	}
	client, err := client.New(config.GRPC.Address, client.WithDefaultNamespace(DefaultNamespace))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to containerd: %w", err)
	}
	return &Cntrd{
		Root:   config.Root,
		Client: client,
	}, nil
}
