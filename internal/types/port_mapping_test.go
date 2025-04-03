package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePortMapping(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMapping PortMapping
		wantErr     bool
		errContains string
	}{
		{
			name:  "Valid TCP mapping",
			input: "8080:80/tcp",
			wantMapping: PortMapping{
				ExternalPort: 8080,
				InternalPort: 80,
				Protocol:     "TCP",
			},
			wantErr: false,
		},
		{
			name:  "Valid UDP mapping",
			input: "1234:5678/udp",
			wantMapping: PortMapping{
				ExternalPort: 1234,
				InternalPort: 5678,
				Protocol:     "UDP",
			},
			wantErr: false,
		},
		{
			name:  "Edge case: min and max ports",
			input: "1:65535/tcp",
			wantMapping: PortMapping{
				ExternalPort: 1,
				InternalPort: 65535,
				Protocol:     "TCP",
			},
			wantErr: false,
		},
		{
			name:        "Missing protocol",
			input:       "8080:80",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid format: expected <external>:<internal>/<protocol>",
		},
		{
			name:        "Invalid protocol",
			input:       "8080:80/xyz",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid protocol: must be TCP or UDP",
		},
		{
			name:        "Missing colon",
			input:       "808080/tcp",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid port format: expected <external>:<internal>",
		},
		{
			name:        "Non-numeric external port",
			input:       "abc:80/tcp",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid external port: must be a number between 1 and 65535",
		},
		{
			name:        "Non-numeric internal port",
			input:       "8080:xyz/tcp",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid internal port: must be a number between 1 and 65535",
		},
		{
			name:        "External port too low",
			input:       "0:80/tcp",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid external port: must be a number between 1 and 65535",
		},
		{
			name:        "External port too high",
			input:       "65536:80/tcp",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid external port: must be a number between 1 and 65535",
		},
		{
			name:        "Internal port too low",
			input:       "8080:0/tcp",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid internal port: must be a number between 1 and 65535",
		},
		{
			name:        "Internal port too high",
			input:       "8080:65536/tcp",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid internal port: must be a number between 1 and 65535",
		},
		{
			name:        "Empty string",
			input:       "",
			wantMapping: PortMapping{},
			wantErr:     true,
			errContains: "Invalid format: expected <external>:<internal>/<protocol>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMapping, err := ParsePortMapping(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantMapping, gotMapping)
				assert.Empty(t, gotMapping.Name)
			}
		})
	}
}
