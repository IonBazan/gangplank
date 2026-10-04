package providers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/portmap"
)

func TestConfigPortProvider_GetPortMappings(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		wantPorts   []portmap.Mapping
		wantErr     bool
		errContains string
	}{
		{
			name: "Valid config with web and stream mappings",
			config: &config.Config{
				Ports: []portmap.Mapping{
					{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "config-web"},
					{ExternalPort: 9000, InternalPort: 90, Protocol: "UDP", Name: "config-stream"},
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "config-web"},
				{ExternalPort: 9000, InternalPort: 90, Protocol: "UDP", Name: "config-stream"},
			},
			wantErr: false,
		},
		{
			name: "Invalid external port",
			config: &config.Config{
				Ports: []portmap.Mapping{
					{ExternalPort: 0, InternalPort: 80, Protocol: "TCP", Name: "invalid-port"},
				},
			},
			wantPorts:   []portmap.Mapping{},
			wantErr:     true,
			errContains: "invalid port mapping at index 0",
		},
		{
			name: "Invalid entry does not drop valid ones",
			config: &config.Config{
				Ports: []portmap.Mapping{
					{ExternalPort: 8080, InternalPort: 80, Protocol: "tcp", Name: "lowercase"},
					{ExternalPort: 9000, InternalPort: 0, Protocol: "UDP", Name: "broken"},
					{ExternalPort: 53, InternalPort: 53, Name: "default-protocol"},
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "lowercase"},
				{ExternalPort: 53, InternalPort: 53, Protocol: "TCP", Name: "default-protocol"},
			},
			wantErr:     true,
			errContains: "invalid port mapping at index 1",
		},
		{
			name: "Invalid protocol",
			config: &config.Config{
				Ports: []portmap.Mapping{
					{ExternalPort: 8080, InternalPort: 80, Protocol: "INVALID", Name: "invalid-protocol"},
				},
			},
			wantPorts:   []portmap.Mapping{},
			wantErr:     true,
			errContains: "invalid port mapping at index 0",
		},
		{
			name:        "Nil config",
			config:      nil,
			wantPorts:   []portmap.Mapping{},
			wantErr:     false,
			errContains: "",
		},
		{
			name: "Empty config ports",
			config: &config.Config{
				Ports: []portmap.Mapping{},
			},
			wantPorts:   []portmap.Mapping{},
			wantErr:     false,
			errContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portProvider := NewConfigPortProvider(tt.config)

			gotPorts, err := portProvider.GetPortMappings(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantPorts, gotPorts)
		})
	}
}
