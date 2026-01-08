package limit

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/kcrow-io/plugins/pkg/containerd"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultWatchInterval is the default interval for watching container stats
	DefaultWatchInterval = 60
	// DefaultMaxDiskBytes is the default max disk bytes
	DefaultMaxDiskBytes = 4 * 1024 * 1024 * 1024 // 4GB

	// DefaultCacheRssRatio is the min cache bytes, will clear when cache > min_cache_bytes.
	DefaultMinCacheBytes = 512 * 1024 * 1024 // 512 MB

	DefaultCacheRssRatio = 10
)

type iolimit struct {
	MaxDiskBytes uint64 `json:"max_disk_bytes,omitempty"`
	BpsLimit     uint64 `json:"bps_limit,omitempty"`
}

func DefaultIoLimit() *iolimit {
	return &iolimit{
		MaxDiskBytes: 4 * 1024 * 1024 * 1024, // 4GB
		BpsLimit:     1,                      // 1bps
	}
}

func (i *iolimit) Valid() error {
	if i.MaxDiskBytes == 0 || i.BpsLimit == 0 {
		return fmt.Errorf("max_disk_bytes must be greater than 0")
	}
	return nil
}

func (i *iolimit) CopyTo(dst *iolimit) {
	dst.MaxDiskBytes = i.MaxDiskBytes
	dst.BpsLimit = i.BpsLimit
}
func (i *iolimit) String() string {
	return fmt.Sprintf("max_disk_bytes: %d, bps_limit: %d", i.MaxDiskBytes, i.BpsLimit)
}

type memlimit struct {
	CacheRssRatio float64 `json:"cache-rss-ratio,omitempty"`
	MinCacheBytes uint64  `json:"min-cache-bytes,omitempty"`
}

func DefaultMemLimit() *memlimit {
	return &memlimit{
		CacheRssRatio: DefaultCacheRssRatio,
		MinCacheBytes: DefaultMinCacheBytes,
	}
}

func (m *memlimit) Valid() error {
	if m.CacheRssRatio < 3 {
		return fmt.Errorf("cache-rss-ratio must be greater than 3")
	}
	return nil
}

func (m *memlimit) CopyTo(dst *memlimit) {
	dst.CacheRssRatio = m.CacheRssRatio
	dst.MinCacheBytes = m.MinCacheBytes
}
func (m *memlimit) String() string {
	return fmt.Sprintf("cache-rss-ratio: %f, min-cache-bytes: %d", m.CacheRssRatio, m.MinCacheBytes)
}

// Config represents the iolimit plugin configuration
type Config struct {
	// ContainerdConfigPath is the path to containerd config file
	ContainerdConfigPath string `json:"containerd_config_path,omitempty"`
	// WatchInterval is the interval in seconds for watching container stats
	WatchInterval int `json:"watch_interval,omitempty"`

	// BpsLimit is the bandwidth limit in bytes per second
	Io     *iolimit  `json:"io,omitempty"`
	Memory *memlimit `json:"memory,omitempty"`

	cntrd *containerd.Cntrd `json:"-"`
}

// NewConfig creates a new Config with default values
func NewConfig() *Config {
	return &Config{
		ContainerdConfigPath: containerd.DefaultContainerdConfigPath,
		Io:                   DefaultIoLimit(),
		Memory:               DefaultMemLimit(),
		WatchInterval:        DefaultWatchInterval,
	}
}

// Parse parses the configuration from JSON bytes
func (c *Config) Parse(data []byte) error {
	if len(data) != 0 {
		tempconfig := &Config{}
		if err := json.Unmarshal(data, tempconfig); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
		}
		if tempconfig.ContainerdConfigPath != "" {
			c.ContainerdConfigPath = tempconfig.ContainerdConfigPath
		}
		if tempconfig.WatchInterval != 0 {
			c.WatchInterval = tempconfig.WatchInterval
		}
		if tempconfig.Io != nil && tempconfig.Io.Valid() == nil {
			tempconfig.Io.CopyTo(c.Io)
		}
		if tempconfig.Memory != nil && tempconfig.Memory.Valid() == nil {
			tempconfig.Memory.CopyTo(c.Memory)
		}
	}

	// Parse containerd config to get root and socket
	cntrd, err := containerd.ParseContainerdConfig(c.ContainerdConfigPath)
	if err != nil {
		// If parsing fails, use defaults and log warning
		logrus.Warnf("Failed to parse containerd config from %s (using defaults): %v", c.ContainerdConfigPath, err)
		return err
	}

	c.cntrd = cntrd

	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
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
