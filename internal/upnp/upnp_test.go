package upnp

import (
	"errors"
	"testing"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestClient_ForwardPorts(t *testing.T) {
	tests := []struct {
		name          string
		mappings      []types.PortMapping
		localIP       string
		forwardErr    error
		wantForwarded []types.PortMapping
		wantErr       bool
	}{
		{
			name: "Single TCP mapping",
			mappings: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
			localIP: "192.168.1.100",
			wantForwarded: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: nginx"},
			},
			wantErr: false,
		},
		{
			name: "Multiple mappings with unnamed port",
			mappings: []types.PortMapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "UDP", Name: ""},
			},
			localIP: "192.168.1.101",
			wantForwarded: []types.PortMapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "Gangplank UPnP: redis"},
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "UDP", Name: "Gangplank UPnP"},
			},
			wantErr: false,
		},
		{
			name:          "Empty mappings",
			mappings:      []types.PortMapping{},
			localIP:       "192.168.1.102",
			wantForwarded: []types.PortMapping{},
			wantErr:       false,
		},
		{
			name: "Mapping with error",
			mappings: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
			localIP:       "192.168.1.103",
			forwardErr:    errors.New("UPnP error"),
			wantForwarded: []types.PortMapping{
				// No mappings should be added due to the error
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &DummyConnection{
				Forwarded:  []types.PortMapping{},
				ForwardErr: tt.forwardErr,
			}
			client := &Client{
				uPnPConnection: mock,
				LocalIP:        tt.localIP,
			}

			err := client.ForwardPorts(tt.mappings)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.forwardErr, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantForwarded, mock.Forwarded)

			if len(tt.mappings) > 0 && !tt.wantErr {
				assert.Len(t, mock.Forwarded, len(tt.mappings))
			}
		})
	}
}

func TestClient_DeletePortMapping(t *testing.T) {
	tests := []struct {
		name      string
		extPort   int
		protocol  string
		deleteErr error
		wantCalls []struct {
			ExtPort  uint16
			Protocol string
		}
		wantErr bool
	}{
		{
			name:     "Delete TCP port",
			extPort:  8080,
			protocol: "TCP",
			wantCalls: []struct {
				ExtPort  uint16
				Protocol string
			}{{ExtPort: 8080, Protocol: "TCP"}},
			wantErr: false,
		},
		{
			name:      "Delete with error",
			extPort:   6379,
			protocol:  "UDP",
			deleteErr: errors.New("delete failed"),
			wantCalls: []struct {
				ExtPort  uint16
				Protocol string
			}{{ExtPort: 6379, Protocol: "UDP"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &DummyConnection{
				Deleted: []struct {
					ExtPort  uint16
					Protocol string
				}{},
				DeleteErr: tt.deleteErr,
			}
			client := &Client{
				uPnPConnection: mock,
				LocalIP:        "192.168.1.100",
			}

			err := client.DeletePortMapping(tt.extPort, tt.protocol)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.deleteErr, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCalls, mock.Deleted)
			}
		})
	}
}

func TestClient_ListPortMappings(t *testing.T) {
	tests := []struct {
		name          string
		expected      []PortMappingEntry
		expectedError error
	}{
		{
			name: "Single mapping",
			expected: []PortMappingEntry{
				{
					ExternalPort:  8080,
					InternalPort:  80,
					Protocol:      "TCP",
					InternalIP:    "192.168.1.100",
					Description:   "Test Mapping",
					LeaseDuration: 3600,
					Enabled:       true,
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &DummyConnection{}

			client := &Client{
				uPnPConnection: mock,
				LocalIP:        "192.168.1.100",
			}

			mappings, err := client.ListPortMappings()
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError.Error(), err.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, mappings)
			}
		})
	}
}
