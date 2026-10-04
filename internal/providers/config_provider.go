package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/portmap"
)

type ConfigPortProvider struct {
	config *config.Config
}

func NewConfigPortProvider(config *config.Config) *ConfigPortProvider {
	return &ConfigPortProvider{config}
}

// Invalid entries are skipped and reported in the returned error.
func (f *ConfigPortProvider) GetPortMappings(_ context.Context) ([]portmap.Mapping, error) {
	if f.config == nil {
		return []portmap.Mapping{}, nil
	}

	mappings := []portmap.Mapping{}
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
