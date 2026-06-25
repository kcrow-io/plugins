package limit

import (
	"encoding/json"
	"fmt"

	"github.com/kcrow-io/plugins/pkg/containerd"
	"github.com/kcrow-io/plugins/pkg/log"
	"github.com/sirupsen/logrus"
)

const (
	// DefaultWatchInterval is the default interval for watching container stats
	DefaultWatchInterval = 60
	// DefaultMaxDiskBytes is the default max disk bytes
	DefaultMaxDiskBytes = 10 * 1024 * 1024 * 1024 // 10GB

	// DefaultCacheRssRatio is the min cache bytes, will clear when cache > min_cache_bytes.
	DefaultMinCacheBytes = 512 * 1024 * 1024 // 512 MB

	DefaultCacheRssRatio = 10
	DefaultUsagePercent  = 80
)

type IoLimit struct {
	MaxDiskBytes uint64 `json:"max_disk_bytes,omitempty"`
	BpsLimit     uint64 `json:"bps_limit,omitempty"`
	IopsLimit    uint64 `json:"iops_limit,omitempty"`
}

func DefaultIoLimit() *IoLimit {
	return &IoLimit{
		MaxDiskBytes: DefaultMaxDiskBytes, // 10GB
		BpsLimit:     4194304,             // 4MB/s
		IopsLimit:    10,                  // 10 IOPS
	}
}

func (i *IoLimit) Valid() error {
	return nil
}

func (i *IoLimit) CopyTo(dst *IoLimit) {
	dst.MaxDiskBytes = i.MaxDiskBytes
	dst.BpsLimit = i.BpsLimit
	dst.IopsLimit = i.IopsLimit
}
func (i *IoLimit) String() string {
	return fmt.Sprintf("max_disk_bytes: %d, bps_limit: %d, iops_limit: %d", i.MaxDiskBytes, i.BpsLimit, i.IopsLimit)
}

type MemLimit struct {
	PodsUsagePercent uint64  `json:"pods-usage-percent,omitempty"`
	CacheRssRatio    float64 `json:"cache-rss-ratio,omitempty"`
	MinCacheBytes    uint64  `json:"min-cache-bytes,omitempty"`
}

func DefaultMemLimit() *MemLimit {
	return &MemLimit{
		PodsUsagePercent: DefaultUsagePercent,
		CacheRssRatio:    DefaultCacheRssRatio,
		MinCacheBytes:    DefaultMinCacheBytes,
	}
}

func (m *MemLimit) Valid() error {
	if m.CacheRssRatio < 3 {
		return fmt.Errorf("cache-rss-ratio must be greater than 3")
	}
	return nil
}

func (m *MemLimit) CopyTo(dst *MemLimit) {
	dst.PodsUsagePercent = m.PodsUsagePercent
	dst.CacheRssRatio = m.CacheRssRatio
	dst.MinCacheBytes = m.MinCacheBytes
}
func (m *MemLimit) String() string {
	return fmt.Sprintf("pods-usage-percent: %d, cache-rss-ratio: %f, min-cache-bytes: %d", m.PodsUsagePercent, m.CacheRssRatio, m.MinCacheBytes)
}

// Config represents the iolimit plugin configuration
type Config struct {
	// Disabled indicates whether the plugin is disabled
	Disabled bool `json:"disabled,omitempty"`
	// ContainerdConfigPath is the path to containerd config file
	ContainerdConfigPath string `json:"containerd_config_path,omitempty"`
	// WatchInterval is the interval in seconds for watching container stats
	WatchInterval int `json:"watch_interval,omitempty"`

	// BpsLimit is the bandwidth limit in bytes per second
	Io     *IoLimit  `json:"io,omitempty"`
	Memory *MemLimit `json:"memory,omitempty"`

	// LogPath is the path to the log file
	LogPath string `json:"log_path,omitempty"`

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
		// Setup file logging if log_path is provided
		if tempconfig.LogPath != "" {
			c.LogPath = tempconfig.LogPath
		}
	}

	// Setup file logging if configured
	if c.LogPath != "" {
		if err := log.SetupFileLogging(c.LogPath); err != nil {
			logrus.Warnf("Failed to setup file logging to %s, continuing with stdout: %v", c.LogPath, err)
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
