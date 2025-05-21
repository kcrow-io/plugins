package escape

import (
	"encoding/json"
	"io"
)

type Config struct {
	Root string `yaml:"root,omitempty"`
}

func DefaultConfig() *Config {
	return &Config{Root: "/system.slice"}
}

func (c *Config) ReadFrom(r io.Reader) (int64, error) {
	var newc = &Config{}
	err := json.NewDecoder(r).Decode(newc)
	if err != nil {
		return 0, err
	}

	if newc.Root == "" {
		newc.Root = c.Root
	}
	c.Root = newc.Root
	return 0, nil
}

func (c *Config) WriteTo(w io.Writer) (int64, error) {
	encode := json.NewEncoder(w)
	encode.SetIndent("", "  ")
	return 0, encode.Encode(c)
}
