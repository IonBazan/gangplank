package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/IonBazan/gangplank/internal/portmap"
)

// DefaultFiles are looked up in the working directory when no file is given.
var DefaultFiles = []string{"gangplank.yaml", "gangplank.yml"}

// legacyFiles were the default files before. They are only reported, never used.
var legacyFiles = []string{"config.yaml", "config.yml"}

type Config struct {
	TTL             *time.Duration    `yaml:"ttl"`
	Gateway         string            `yaml:"gateway"`
	LocalIP         string            `yaml:"localIp"`
	RefreshInterval time.Duration     `yaml:"refreshInterval"`
	Ports           []portmap.Mapping `yaml:"ports"`
}

// Load reads the given file. With an empty path it tries DefaultFiles and
// returns nil without an error when none of them exists, unless a legacy
// config.yaml with Gangplank settings is found.
func Load(path string) (*Config, error) {
	if path != "" {
		return read(path)
	}

	for _, name := range DefaultFiles {
		cfg, err := read(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		return cfg, err
	}

	// A file that is not a valid Gangplank config belongs to something else.
	for _, name := range legacyFiles {
		if _, err := read(name); err == nil {
			return nil, fmt.Errorf("%s is no longer read by default: rename it to gangplank.yaml or pass it with --config", name)
		}
	}

	return nil, nil
}

func read(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return &cfg, nil
}

// FlagValues returns the settings that were set, keyed by command-line flag name.
func (c *Config) FlagValues() map[string]string {
	values := map[string]string{}
	if c == nil {
		return values
	}
	if c.TTL != nil {
		values["ttl"] = c.TTL.String()
	}
	if c.Gateway != "" {
		values["gateway"] = c.Gateway
	}
	if c.LocalIP != "" {
		values["local-ip"] = c.LocalIP
	}
	if c.RefreshInterval > 0 {
		values["refresh-interval"] = c.RefreshInterval.String()
	}

	return values
}
