package fetchers

import (
	"fmt"
	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/types"
)

type ConfigPortFetcher struct {
	config *config.Config
}

func NewConfigPortFetcher(config *config.Config) *ConfigPortFetcher {
	return &ConfigPortFetcher{config}
}

func (f *ConfigPortFetcher) FetchPorts() ([]types.PortMapping, error) {
	if f.config == nil {
		return []types.PortMapping{}, nil
	}

	for i, p := range f.config.Ports {
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("invalid port mapping at index %d: %v", i, err)
		}
	}

	return f.config.Ports, nil
}
