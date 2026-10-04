package portmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePortMapping(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantMapping Mapping
		wantErr     bool
		errContains string
	}{
		{
			name:  "Valid TCP mapping",
			input: "8080:80/tcp",
			wantMapping: Mapping{
				ExternalPort: 8080,
				InternalPort: 80,
				Protocol:     "TCP",
			},
			wantErr: false,
		},
		{
			name:  "Valid UDP mapping",
			input: "1234:5678/udp",
			wantMapping: Mapping{
				ExternalPort: 1234,
				InternalPort: 5678,
				Protocol:     "UDP",
			},
			wantErr: false,
		},
		{
			name:  "Valid mapping without protocol",
			input: "8080:80",
			wantMapping: Mapping{
				ExternalPort: 8080,
				InternalPort: 80,
				Protocol:     "TCP",
			},
			wantErr: false,
		},
		{
			name:  "Valid mapping with single port",
			input: "80",
			wantMapping: Mapping{
				ExternalPort: 80,
				InternalPort: 80,
				Protocol:     "TCP",
			},
			wantErr: false,
		},
		{
			name:  "Valid mapping with single port and protocol",
			input: ":80/udp",
			wantMapping: Mapping{
				ExternalPort: 80,
				InternalPort: 80,
				Protocol:     "UDP",
			},
			wantErr: false,
		},
		{
			name:  "Edge case: min and max ports",
			input: "1:65535/tcp",
			wantMapping: Mapping{
				ExternalPort: 1,
				InternalPort: 65535,
				Protocol:     "TCP",
			},
			wantErr: false,
		},
		{
			name:        "Invalid protocol",
			input:       "8080:80/xyz",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "protocol must be 'TCP' or 'UDP'",
		},
		{
			name:        "Non-numeric external port",
			input:       "abc:80/tcp",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "external port must be a number between 1 and 65535",
		},
		{
			name:        "Non-numeric internal port",
			input:       "8080:xyz/tcp",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "internal port must be a number between 1 and 65535",
		},
		{
			name:        "External port too low",
			input:       "0:80/tcp",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "external port must be a number between 1 and 65535",
		},
		{
			name:        "External port too high",
			input:       "65536:80/tcp",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "external port must be a number between 1 and 65535",
		},
		{
			name:        "Internal port too low",
			input:       "8080:0/tcp",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "internal port must be a number between 1 and 65535",
		},
		{
			name:        "Internal port too high",
			input:       "8080:65536/tcp",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "internal port must be a number between 1 and 65535",
		},
		{
			name:        "Empty string",
			input:       "",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "invalid port format: port cannot be empty",
		},
		{
			name:        "Colon only",
			input:       ":",
			wantMapping: Mapping{},
			wantErr:     true,
			errContains: "invalid port format: both external and internal ports cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMapping, err := Parse(tt.input)
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

func TestPortMapping_Normalize(t *testing.T) {
	assert.Equal(t, "TCP", Mapping{}.Normalize().Protocol)
	assert.Equal(t, "UDP", Mapping{Protocol: " udp "}.Normalize().Protocol)
	assert.Equal(t, "8080/TCP", Mapping{ExternalPort: 8080, Protocol: "tcp"}.Normalize().Key())
}

func TestParsePortMapping_EdgeCases(t *testing.T) {
	tests := []struct {
		input       string
		want        Mapping
		errContains string
	}{
		{input: " 8080 : 80 / tcp ", want: Mapping{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP"}},
		{input: "8080: 80/udp", want: Mapping{ExternalPort: 8080, InternalPort: 80, Protocol: "UDP"}},
		{input: "80/TCP", want: Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}},
		{input: "65535", want: Mapping{ExternalPort: 65535, InternalPort: 65535, Protocol: "TCP"}},
		{input: "1:2:3", errContains: "invalid port format"},
		{input: "80/tcp/udp", errContains: "invalid format"},
		{input: "80/", errContains: "protocol must be"},
		{input: "-1", errContains: "port must be a number"},
		{input: "abc", errContains: "port must be a number"},
		{input: "8080:", want: Mapping{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP"}},
		{input: ":8080/udp", want: Mapping{ExternalPort: 8080, InternalPort: 8080, Protocol: "UDP"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := Parse(tt.input)
			if tt.errContains != "" {
				assert.ErrorContains(t, err, tt.errContains)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPortMapping_Validate(t *testing.T) {
	tests := []struct {
		name        string
		mapping     Mapping
		errContains string
	}{
		{name: "Valid lowercase protocol", mapping: Mapping{ExternalPort: 1, InternalPort: 65535, Protocol: "udp"}},
		{name: "External too high", mapping: Mapping{ExternalPort: 70000, InternalPort: 80, Protocol: "TCP"}, errContains: "external port"},
		{name: "Internal missing", mapping: Mapping{ExternalPort: 80, Protocol: "TCP"}, errContains: "internal port"},
		{name: "Empty protocol", mapping: Mapping{ExternalPort: 80, InternalPort: 80}, errContains: "protocol must be"},
		{name: "SCTP is not supported", mapping: Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "sctp"}, errContains: "protocol must be"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mapping.Validate()
			if tt.errContains == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.errContains)
			}
		})
	}
}
