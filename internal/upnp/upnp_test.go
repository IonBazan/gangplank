package upnp

import (
	"errors"
	"testing"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/stretchr/testify/assert"
)

type MockUPnPClient struct {
	AddCalls []struct {
		ExtPort, IntPort   uint16
		Protocol, IP, Desc string
	}
	DeleteCalls []struct {
		ExtPort  uint16
		Protocol string
	}
	AddErr    error
	DeleteErr error
	ExtIP     string
}

func (m *MockUPnPClient) GetExternalIPAddress() (string, error) {
	return m.ExtIP, nil
}

func (m *MockUPnPClient) AddPortMapping(
	NewRemoteHost string,
	NewExternalPort uint16,
	NewProtocol string,
	NewInternalPort uint16,
	NewInternalClient string,
	NewEnabled bool,
	NewPortMappingDescription string,
	NewLeaseDuration uint32,
) error {
	m.AddCalls = append(m.AddCalls, struct {
		ExtPort, IntPort   uint16
		Protocol, IP, Desc string
	}{
		ExtPort:  NewExternalPort,
		IntPort:  NewInternalPort,
		Protocol: NewProtocol,
		IP:       NewInternalClient,
		Desc:     NewPortMappingDescription,
	})
	return m.AddErr
}

func (m *MockUPnPClient) DeletePortMapping(
	NewRemoteHost string,
	NewExternalPort uint16,
	NewProtocol string,
) error {
	m.DeleteCalls = append(m.DeleteCalls, struct {
		ExtPort  uint16
		Protocol string
	}{
		ExtPort:  NewExternalPort,
		Protocol: NewProtocol,
	})
	return m.DeleteErr
}

func TestClient_ForwardPorts(t *testing.T) {
	tests := []struct {
		name      string
		mappings  []types.PortMapping
		localIP   string
		addErr    error
		wantCalls []struct {
			ExtPort, IntPort   uint16
			Protocol, IP, Desc string
		}
	}{
		{
			name: "Single TCP mapping",
			mappings: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
			localIP: "192.168.1.100",
			wantCalls: []struct {
				ExtPort, IntPort   uint16
				Protocol, IP, Desc string
			}{
				{ExtPort: 8080, IntPort: 80, Protocol: "TCP", IP: "192.168.1.100", Desc: "Gangplank UPnP: nginx"},
			},
		},
		{
			name: "Multiple mappings with unnamed port",
			mappings: []types.PortMapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "UDP", Name: ""},
			},
			localIP: "192.168.1.101",
			wantCalls: []struct {
				ExtPort, IntPort   uint16
				Protocol, IP, Desc string
			}{
				{ExtPort: 6379, IntPort: 6379, Protocol: "TCP", IP: "192.168.1.101", Desc: "Gangplank UPnP: redis"},
				{ExtPort: 5433, IntPort: 5432, Protocol: "UDP", IP: "192.168.1.101", Desc: "Gangplank UPnP"},
			},
		},
		{
			name:     "Empty mappings",
			mappings: []types.PortMapping{},
			localIP:  "192.168.1.102",
			wantCalls: []struct {
				ExtPort, IntPort   uint16
				Protocol, IP, Desc string
			}{},
		},
		{
			name: "Mapping with error",
			mappings: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
			localIP: "192.168.1.103",
			addErr:  errors.New("UPnP error"),
			wantCalls: []struct {
				ExtPort, IntPort   uint16
				Protocol, IP, Desc string
			}{
				{ExtPort: 8080, IntPort: 80, Protocol: "TCP", IP: "192.168.1.103", Desc: "Gangplank UPnP: nginx"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &MockUPnPClient{
				AddCalls: []struct {
					ExtPort, IntPort   uint16
					Protocol, IP, Desc string
				}{},
				AddErr: tt.addErr,
			}
			client := &Client{
				upnpClient: mock,
				LocalIP:    tt.localIP,
			}

			err := client.ForwardPorts(tt.mappings)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantCalls, mock.AddCalls)

			if len(tt.mappings) > 0 {
				assert.Len(t, mock.AddCalls, len(tt.mappings))
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
			mock := &MockUPnPClient{
				DeleteCalls: []struct {
					ExtPort  uint16
					Protocol string
				}{},
				DeleteErr: tt.deleteErr,
			}
			client := &Client{
				upnpClient: mock,
				LocalIP:    "192.168.1.100",
			}

			err := client.DeletePortMapping(tt.extPort, tt.protocol)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Equal(t, tt.deleteErr, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalls, mock.DeleteCalls)
		})
	}
}
