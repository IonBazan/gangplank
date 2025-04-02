package config

import (
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/spf13/viper"
	"time"
)

type Config struct {
	Duration        time.Duration       `mapstructure:"duration" yaml:"duration"`
	Gateway         string              `mapstructure:"gateway" yaml:"gateway"`
	LocalIP         string              `mapstructure:"localIp" yaml:"localIp"`
	RefreshInterval time.Duration       `mapstructure:"refreshInterval" yaml:"refreshInterval"`
	Ports           []types.PortMapping `mapstructure:"ports" yaml:"ports"`
}

func LoadConfig(configPath string) (*Config, error) {
	v := viper.New()

	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Set default locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, err
	}

	return &config, nil
}
