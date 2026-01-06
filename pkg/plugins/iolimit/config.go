package iolimit

import (
	"encoding/json"
	"fmt"
	"io"
)

const (
	// DefaultContainerdSocket is the default containerd socket path
	DefaultContainerdSocket = "/run/containerd/containerd.sock"
	// DefaultContainerdRoot is the default containerd root directory
	DefaultContainerdRoot = "/var/lib/containerd"
	// DefaultMaxDiskBytes is the default disk threshold (4GB)
	DefaultMaxDiskBytes = 4 * 1024 * 1024 * 1024
	// DefaultBpsLimit is the default bandwidth limit (1kbps)
	DefaultBpsLimit = 1024
	// DefaultWatchInterval is the default interval for watching container stats
	DefaultWatchInterval = 60

	// DefaultSnapshotter is the default snapshotter
	DefaultSnapshotter = "overlayfs"

	DefaultNamespace = "k8s.io"
)

// Config represents the iolimit plugin configuration
type Config struct {
	// ContainerdSocket is the path to containerd socket
	ContainerdSocket string `json:"containerd_socket,omitempty"`
	// ContainerdRoot is the containerd root directory
	ContainerdRoot string `json:"containerd_root,omitempty"`
	// MaxDiskBytes is the disk usage threshold in bytes
	MaxDiskBytes int64 `json:"max_disk_bytes,omitempty"`
	// BpsLimit is the bandwidth limit in bytes per second
	BpsLimit int64 `json:"bps_limit,omitempty"`
	// WatchInterval is the interval in seconds for watching container stats
	WatchInterval int `json:"watch_interval,omitempty"`
}

// NewConfig creates a new Config with default values
func NewConfig() *Config {
	return &Config{
		ContainerdSocket: DefaultContainerdSocket,
		ContainerdRoot:   DefaultContainerdRoot,
		MaxDiskBytes:     DefaultMaxDiskBytes,
		BpsLimit:         DefaultBpsLimit,
		WatchInterval:    DefaultWatchInterval,
	}
}

// Parse parses the configuration from JSON bytes
func (c *Config) Parse(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults if not set
	if c.ContainerdSocket == "" {
		c.ContainerdSocket = DefaultContainerdSocket
	}
	if c.ContainerdRoot == "" {
		c.ContainerdRoot = DefaultContainerdRoot
	}
	if c.MaxDiskBytes == 0 {
		c.MaxDiskBytes = DefaultMaxDiskBytes
	}
	if c.BpsLimit == 0 {
		c.BpsLimit = DefaultBpsLimit
	}
	if c.WatchInterval == 0 {
		c.WatchInterval = DefaultWatchInterval
	}

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.MaxDiskBytes <= 0 {
		return fmt.Errorf("max_disk_bytes must be positive")
	}
	if c.BpsLimit <= 0 {
		return fmt.Errorf("bps_limit must be positive")
	}
	if c.WatchInterval <= 0 {
		return fmt.Errorf("watch_interval must be positive")
	}
	return nil
}

// ReadFrom implements the Configer interface
func (c *Config) ReadFrom(r io.Reader) (int64, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}

	if err := json.Unmarshal(data, c); err != nil {
		return 0, err
	}

	return int64(len(data)), nil
}

// WriteTo implements the Configer interface
func (c *Config) WriteTo(w io.Writer) (int64, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return 0, err
	}

	n, err := w.Write(data)
	return int64(n), err
}
