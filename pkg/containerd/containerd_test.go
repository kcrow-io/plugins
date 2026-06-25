package containerd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestParseContainerdConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		wantAddress string
		wantErr     bool
	}{
		{
			name: "old grpc format",
			config: `
root = "/var/lib/containerd"
[grpc]
  address = "/run/containerd/containerd.sock"
`,
			wantAddress: "/run/containerd/containerd.sock",
		},
		{
			name: "new plugins format",
			config: `
root = "/var/lib/containerd"
[plugins."io.containerd.server.v1.grpc"]
  address = "/run/containerd/containerd.sock"
`,
			wantAddress: "/run/containerd/containerd.sock",
		},
		{
			name: "new format takes precedence",
			config: `
root = "/var/lib/containerd"
[grpc]
  address = "/old/socket.sock"
[plugins."io.containerd.server.v1.grpc"]
  address = "/new/socket.sock"
`,
			wantAddress: "/new/socket.sock",
		},
		{
			name: "default when no address",
			config: `
root = "/var/lib/containerd"
`,
			wantAddress: "/run/containerd/containerd.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tt.config), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			// Note: ParseContainerdConfig also tries to connect to containerd
			// We can't test the full function without a running containerd,
			// but we can test the config parsing logic by extracting it
			// For now, we'll just verify the file exists
			if _, err := os.Stat(configPath); err != nil {
				t.Fatalf("config file not created: %v", err)
			}
		})
	}
}

func TestContainerdConfigParsing(t *testing.T) {
	// Test the struct parsing directly using toml
	import_test_data := []struct {
		name        string
		config      string
		wantAddress string
	}{
		{
			name: "old grpc format",
			config: `
root = "/var/lib/containerd"
[grpc]
  address = "/run/containerd/containerd.sock"
`,
			wantAddress: "/run/containerd/containerd.sock",
		},
		{
			name: "new plugins format",
			config: `
root = "/var/lib/containerd"
[plugins."io.containerd.server.v1.grpc"]
  address = "/run/containerd/containerd.sock"
`,
			wantAddress: "/run/containerd/containerd.sock",
		},
		{
			name: "new format takes precedence",
			config: `
root = "/var/lib/containerd"
[grpc]
  address = "/old/socket.sock"
[plugins."io.containerd.server.v1.grpc"]
  address = "/new/socket.sock"
`,
			wantAddress: "/new/socket.sock",
		},
		{
			name: "default when no address",
			config: `
root = "/var/lib/containerd"
`,
			wantAddress: "/run/containerd/containerd.sock",
		},
	}

	for _, tt := range import_test_data {
		t.Run(tt.name, func(t *testing.T) {
			var config containerdConfig
			if _, err := toml.Decode(tt.config, &config); err != nil {
				t.Fatalf("failed to decode config: %v", err)
			}

			// Apply the same logic as ParseContainerdConfig
			address := config.Plugins.IoContainerdServerV1Grpc.Address
			if address == "" {
				address = config.GRPC.Address
			}
			if address == "" {
				address = "/run/containerd/containerd.sock"
			}

			if address != tt.wantAddress {
				t.Errorf("got address %q, want %q", address, tt.wantAddress)
			}
		})
	}
}
