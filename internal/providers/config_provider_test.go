package providers

import (
	"testing"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/stretchr/testify/assert"
)

func TestConfigPortProvider_GetPortMappings(t *testing.T) {
	tests := []struct {
		name        string
		config      *config.Config
		wantPorts   []types.PortMapping
		wantErr     bool
		errContains string
	}{
		{
			name: "Valid config with web and stream mappings",
			config: &config.Config{
				Ports: []types.PortMapping{
					{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "config-web"},
					{ExternalPort: 9000, InternalPort: 90, Protocol: "UDP", Name: "config-stream"},
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "config-web"},
				{ExternalPort: 9000, InternalPort: 90, Protocol: "UDP", Name: "config-stream"},
			},
			wantErr: false,
		},
		{
			name: "Invalid external port",
			config: &config.Config{
				Ports: []types.PortMapping{
					{ExternalPort: 0, InternalPort: 80, Protocol: "TCP", Name: "invalid-port"},
				},
			},
			wantPorts:   nil,
			wantErr:     true,
			errContains: "invalid port mapping at index 0",
		},
		{
			name: "Invalid protocol",
			config: &config.Config{
				Ports: []types.PortMapping{
					{ExternalPort: 8080, InternalPort: 80, Protocol: "INVALID", Name: "invalid-protocol"},
				},
			},
			wantPorts:   nil,
			wantErr:     true,
			errContains: "invalid port mapping at index 0",
		},
		{
			name:        "Nil config",
			config:      nil,
			wantPorts:   []types.PortMapping{},
			wantErr:     false,
			errContains: "",
		},
		{
			name: "Empty config ports",
			config: &config.Config{
				Ports: []types.PortMapping{},
			},
			wantPorts:   []types.PortMapping{},
			wantErr:     false,
			errContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			portProvider := NewConfigPortProvider(tt.config)

			gotPorts, err := portProvider.GetPortMappings()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, gotPorts)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPorts, gotPorts)
			}

			if !tt.wantErr && len(gotPorts) > 0 {
				client := upnp.NewDummyClient(upnp.DefaultLeaseDuration)
				err := client.ForwardPorts(gotPorts)
				assert.NoError(t, err, "Dummy client should not fail")
			}
		})
	}
}
