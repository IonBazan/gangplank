package providers

import (
	"fmt"
	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/types"
)

type CofingPortProvider struct {
	config *config.Config
}

func NewConfigPortProvider(config *config.Config) *CofingPortProvider {
	return &CofingPortProvider{config}
}

func (f *CofingPortProvider) GetPortMappings() ([]types.PortMapping, error) {
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
