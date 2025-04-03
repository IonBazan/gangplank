package internal

import (
	"context"
	"errors"
	"github.com/IonBazan/gangplank/internal/providers"
	"testing"
	"time"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/stretchr/testify/assert"
)

type MockPortProvider struct {
	Ports []types.PortMapping
	Err   error
}

func (m *MockPortProvider) GetPortMappings() ([]types.PortMapping, error) {
	return m.Ports, m.Err
}

type MockEventPortProvider struct {
	AddCh    chan types.PortMapping
	DeleteCh chan types.PortMapping
}

func (m *MockEventPortProvider) Listen(ctx context.Context, events providers.PortEventChannels) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case p := <-m.AddCh:
				events.Add <- p
			case p := <-m.DeleteCh:
				events.Delete <- p
			}
		}
	}()
}

type MockUPnPConnection struct {
	Forwarded []types.PortMapping
	Deleted   []struct {
		ExtPort  uint16
		Protocol string
	}
	ForwardErr error
	DeleteErr  error
}

func (m *MockUPnPConnection) GetExternalIPAddress() (string, error) {
	return "192.168.1.1", nil
}

func (m *MockUPnPConnection) AddPortMapping(
	NewRemoteHost string,
	NewExternalPort uint16,
	NewProtocol string,
	NewInternalPort uint16,
	NewInternalClient string,
	NewEnabled bool,
	NewPortMappingDescription string,
	NewLeaseDuration uint32,
) error {
	if m.ForwardErr != nil {
		return m.ForwardErr
	}
	m.Forwarded = append(m.Forwarded, types.PortMapping{
		ExternalPort: int(NewExternalPort),
		InternalPort: int(NewInternalPort),
		Protocol:     NewProtocol,
		Name:         NewPortMappingDescription,
	})
	return nil
}

func (m *MockUPnPConnection) DeletePortMapping(
	NewRemoteHost string,
	NewExternalPort uint16,
	NewProtocol string,
) error {
	if m.DeleteErr != nil {
		return m.DeleteErr
	}
	m.Deleted = append(m.Deleted, struct {
		ExtPort  uint16
		Protocol string
	}{NewExternalPort, NewProtocol})
	return nil
}

func TestGangplank_GetPortMappings(t *testing.T) {
	tests := []struct {
		name          string
		portProviders []providers.PortProvider
		wantPorts     []types.PortMapping
		wantErr       bool
	}{
		{
			name: "Multiple PortProviders",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Ports: []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}}},
				&MockPortProvider{Ports: []types.PortMapping{{ExternalPort: 5432, InternalPort: 5432, Protocol: "TCP", Name: "db"}}},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"},
				{ExternalPort: 5432, InternalPort: 5432, Protocol: "TCP", Name: "db"},
			},
		},
		{
			name:          "Empty PortProviders",
			portProviders: []providers.PortProvider{},
			wantPorts:     []types.PortMapping{},
		},
		{
			name: "Provider error",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Err: errors.New("Get port mappings error")},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Gangplank{
				PortProviders: tt.portProviders,
			}
			ports, err := g.GetPortMappings()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantPorts, ports)
			}
		})
	}
}

func TestGangplank_ForwardPorts(t *testing.T) {
	tests := []struct {
		name       string
		upnpClient *upnp.Client
		ports      []types.PortMapping
		wantErr    bool
		wantMapped []types.PortMapping
	}{
		{
			name:  "No UPnP client",
			ports: []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}},
		},
		{
			name:       "Forward ports successfully",
			ports:      []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}},
			wantMapped: []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: web"}},
		},
		{
			name:    "Forward with error",
			ports:   []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnection := &MockUPnPConnection{}
			var upnpClient *upnp.Client
			if tt.name != "No UPnP client" {
				if tt.wantErr {
					mockConnection.ForwardErr = errors.New("forward error")
				}
				upnpClient = upnp.NewClientWithConnection(mockConnection, "192.168.1.100", upnp.DefaultLeaseDuration)
			}

			g := &Gangplank{
				upnpClient: upnpClient,
			}
			err := g.ForwardPorts(tt.ports)
			assert.NoError(t, err)
			if tt.upnpClient != nil {
				assert.Equal(t, tt.wantMapped, mockConnection.Forwarded)
			}
		})
	}
}

func TestGangplank_PollAndForward(t *testing.T) {
	tests := []struct {
		name        string
		upnpClient  *upnp.Client
		cleanup     bool
		addEvents   []types.PortMapping
		delEvents   []types.PortMapping
		wantAdded   []types.PortMapping
		wantDeleted []struct {
			ExtPort  uint16
			Protocol string
		}
	}{
		{
			name:      "Poll without UPnP",
			addEvents: []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}},
		},
		{
			name:      "Poll and forward",
			addEvents: []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}},
			wantAdded: []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: web"}},
		},
		{
			name:      "Poll with cleanup",
			cleanup:   true,
			delEvents: []types.PortMapping{{ExternalPort: 25565, InternalPort: 25565, Protocol: "TCP", Name: "minecraft"}},
			wantDeleted: []struct {
				ExtPort  uint16
				Protocol string
			}{{ExtPort: 25565, Protocol: "TCP"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConnection := &MockUPnPConnection{}
			var upnpClient *upnp.Client
			if tt.name != "Poll without UPnP" {
				upnpClient = upnp.NewClientWithConnection(mockConnection, "192.168.1.100", upnp.DefaultLeaseDuration)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			eventPortProvider := &MockEventPortProvider{
				AddCh:    make(chan types.PortMapping, len(tt.addEvents)),
				DeleteCh: make(chan types.PortMapping, len(tt.delEvents)),
			}

			for _, p := range tt.addEvents {
				eventPortProvider.AddCh <- p
			}
			for _, p := range tt.delEvents {
				eventPortProvider.DeleteCh <- p
			}

			g := &Gangplank{
				EventPortProviders: []providers.EventPortProvider{eventPortProvider},
				upnpClient:         upnpClient,
			}

			go g.PollAndForward(ctx, tt.cleanup)

			time.Sleep(500 * time.Millisecond)

			cancel()

			if tt.upnpClient != nil {
				assert.Equal(t, tt.wantAdded, mockConnection.Forwarded)
				assert.Equal(t, tt.wantDeleted, mockConnection.Deleted)
			}
		})
	}
}
