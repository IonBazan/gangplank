package fetchers

import (
	"os"
	"testing"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/stretchr/testify/assert"
)

func TestYamlPortFetcher_FetchPorts(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		filename    string
		wantPorts   []types.PortMapping
		wantErr     bool
		errContains string
	}{
		{
			name: "Valid config with web and stream mappings",
			config: `
ports:
  - externalPort: 8080
    internalPort: 80
    protocol: TCP
    name: "yaml-web"
  - externalPort: 9000
    internalPort: 90
    protocol: UDP
    name: "yaml-stream"
`,
			filename: "test_config.yaml",
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "yaml-web"},
				{ExternalPort: 9000, InternalPort: 90, Protocol: "UDP", Name: "yaml-stream"},
			},
			wantErr: false,
		},
		{
			name: "Invalid YAML syntax",
			config: `
ports:
  - externalPort: 8080
    internalPort: 80
    protocol: TCP
    name: "yaml-web"
  - externalPort 9000  # Missing colon
    internalPort: 90
    protocol: UDP
    name: "yaml-stream"
`,
			filename:    "test_invalid.yaml",
			wantPorts:   nil,
			wantErr:     true,
			errContains: "yaml: line",
		},
		{
			name:        "Missing file",
			config:      "",
			filename:    "nonexistent.yaml",
			wantPorts:   nil,
			wantErr:     true,
			errContains: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config != "" {
				err := os.WriteFile(tt.filename, []byte(tt.config), 0644)
				assert.NoError(t, err, "Failed to write test config file")
				defer os.Remove(tt.filename)
			}

			fetcher := NewYamlPortFetcher(tt.filename)

			gotPorts, err := fetcher.FetchPorts()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				assert.Nil(t, gotPorts)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPorts, gotPorts)
			}

			if !tt.wantErr && len(gotPorts) > 0 {
				client, err := upnp.NewDummyClient(upnp.DefaultLeaseDuration)
				assert.NoError(t, err)
				err = client.ForwardPorts(gotPorts)
				assert.NoError(t, err, "Dummy client should not fail")
			}
		})
	}
}
