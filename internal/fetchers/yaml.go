package fetchers

import (
	"fmt"
	"os"

	gangplanktypes "github.com/IonBazan/gangplank/internal/types"

	"gopkg.in/yaml.v3"
)

type YamlPortFetcher struct {
	configPath string
}

func NewYamlPortFetcher(configPath string) *YamlPortFetcher {
	return &YamlPortFetcher{configPath: configPath}
}

func (y *YamlPortFetcher) FetchPorts() ([]gangplanktypes.PortMapping, error) {
	data, err := os.ReadFile(y.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML config file %s: %v", y.configPath, err)
	}

	var config struct {
		Ports []gangplanktypes.PortMapping `yaml:"ports"`
	}

	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config from %s: %v", y.configPath, err)
	}

	for i, p := range config.Ports {
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("invalid port mapping at index %d in %s: %v", i, y.configPath, err)
		}
	}

	return config.Ports, nil
}
