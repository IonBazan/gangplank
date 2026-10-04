package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/types"
)

type ConfigPortProvider struct {
	config *config.Config
}

func NewConfigPortProvider(config *config.Config) *ConfigPortProvider {
	return &ConfigPortProvider{config}
}

// GetPortMappings returns the valid mappings from the config file. Invalid
// entries are skipped and reported in the returned error.
func (f *ConfigPortProvider) GetPortMappings(_ context.Context) ([]types.PortMapping, error) {
	if f.config == nil {
		return []types.PortMapping{}, nil
	}

	mappings := []types.PortMapping{}
	var errs []error
	for i, p := range f.config.Ports {
		p = p.Normalize()
		if err := p.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("invalid port mapping at index %d: %w", i, err))
			continue
		}
		mappings = append(mappings, p)
	}

	return mappings, errors.Join(errs...)
}
