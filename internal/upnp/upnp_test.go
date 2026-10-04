package upnp

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/IonBazan/gangplank/internal/portmap"
)

func TestClient_ForwardPorts(t *testing.T) {
	tests := []struct {
		name          string
		mappings      []portmap.Mapping
		localIP       string
		forwardErr    error
		wantForwarded []portmap.Mapping
		wantErr       bool
	}{
		{
			name: "Single TCP mapping",
			mappings: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
			localIP: "192.168.1.100",
			wantForwarded: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: nginx"},
			},
		},
		{
			name: "Multiple mappings with unnamed port and lowercase protocol",
			mappings: []portmap.Mapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "udp", Name: ""},
			},
			localIP: "192.168.1.101",
			wantForwarded: []portmap.Mapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "Gangplank UPnP: redis"},
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "UDP", Name: "Gangplank UPnP"},
			},
		},
		{
			name:          "Empty mappings",
			mappings:      []portmap.Mapping{},
			localIP:       "192.168.1.102",
			wantForwarded: nil,
		},
		{
			name: "Mapping with error",
			mappings: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
				{ExternalPort: 8081, InternalPort: 81, Protocol: "TCP", Name: "other"},
			},
			localIP:       "192.168.1.103",
			forwardErr:    errors.New("UPnP error"),
			wantForwarded: nil,
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &DummyConnection{ForwardErr: tt.forwardErr}
			client := NewClientWithConnection(mock, tt.localIP, DefaultLeaseDuration)

			err := client.ForwardPorts(context.Background(), tt.mappings)
			if tt.wantErr {
				assert.ErrorIs(t, err, tt.forwardErr)
				// Every mapping is attempted and reported, not just the first.
				assert.Contains(t, err.Error(), "8080/TCP")
				assert.Contains(t, err.Error(), "8081/TCP")
			} else {
				assert.NoError(t, err)
			}
			forwarded, _ := mock.Snapshot()
			assert.Equal(t, tt.wantForwarded, forwarded)
		})
	}
}

type permanentOnlyConnection struct {
	DummyConnection
	leases []uint32
}

func (c *permanentOnlyConnection) AddPortMappingCtx(ctx context.Context, host string, ext uint16, proto string, internal uint16, client string, enabled bool, desc string, lease uint32) error {
	c.leases = append(c.leases, lease)
	if lease != 0 {
		return NewUPnPError(errCodeOnlyPermanentLeasesSupported, "OnlyPermanentLeasesSupported")
	}
	return nil
}

func TestClient_ForwardPorts_PermanentLeaseFallback(t *testing.T) {
	conn := &permanentOnlyConnection{}
	client := NewClientWithConnection(conn, "192.168.1.100", DefaultLeaseDuration)

	ports := []portmap.Mapping{
		{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"},
		{ExternalPort: 443, InternalPort: 443, Protocol: "TCP"},
	}
	assert.NoError(t, client.ForwardPorts(context.Background(), ports))
	// First call is rejected and retried, later calls go straight to a permanent lease.
	assert.Equal(t, []uint32{3600, 0, 0}, conn.leases)
}

func TestClient_DeletePortMapping(t *testing.T) {
	tests := []struct {
		name      string
		extPort   int
		protocol  string
		deleteErr error
		wantCalls []DeletedMapping
		wantErr   bool
	}{
		{
			name:      "Delete TCP port",
			extPort:   8080,
			protocol:  "tcp",
			wantCalls: []DeletedMapping{{ExtPort: 8080, Protocol: "TCP"}},
		},
		{
			name:      "Delete with error",
			extPort:   6379,
			protocol:  "UDP",
			deleteErr: errors.New("delete failed"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &DummyConnection{DeleteErr: tt.deleteErr}
			client := NewClientWithConnection(mock, "192.168.1.100", DefaultLeaseDuration)

			err := client.DeletePortMapping(context.Background(), tt.extPort, tt.protocol)
			if tt.wantErr {
				assert.Equal(t, tt.deleteErr, err)
			} else {
				assert.NoError(t, err)
				_, deleted := mock.Snapshot()
				assert.Equal(t, tt.wantCalls, deleted)
			}
		})
	}
}

type failingListConnection struct {
	DummyConnection
	err error
}

func (c *failingListConnection) GetGenericPortMappingEntryCtx(ctx context.Context, index uint16) (string, uint16, string, uint16, string, bool, string, uint32, error) {
	if index == 0 {
		return "", 80, "TCP", 80, "192.168.1.100", true, "first", 0, nil
	}
	return "", 0, "", 0, "", false, "", 0, c.err
}

func TestClient_ListPortMappings(t *testing.T) {
	first := PortMappingEntry{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", InternalIP: "192.168.1.100", Description: "first", Enabled: true}

	tests := []struct {
		name    string
		conn    UPnPConnection
		want    []PortMappingEntry
		wantErr bool
	}{
		{
			name: "Single mapping",
			conn: &DummyConnection{},
			want: []PortMappingEntry{{
				ExternalPort:  8080,
				InternalPort:  80,
				Protocol:      "TCP",
				InternalIP:    "192.168.1.100",
				Description:   "Test Mapping",
				LeaseDuration: 3600,
				Enabled:       true,
			}},
		},
		{
			name: "End of list signalled with 713",
			conn: &failingListConnection{err: NewUPnPError(713, "SpecifiedArrayIndexInvalid")},
			want: []PortMappingEntry{first},
		},
		{
			name: "End of list signalled with 714",
			conn: &failingListConnection{err: NewUPnPError(714, "NoSuchEntryInArray")},
			want: []PortMappingEntry{first},
		},
		{
			name:    "Other error",
			conn:    &failingListConnection{err: errors.New("boom")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClientWithConnection(tt.conn, "192.168.1.100", DefaultLeaseDuration)

			mappings, err := client.ListPortMappings(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, mappings)
			}
		})
	}
}

func TestPortMappingEntry_IsOwned(t *testing.T) {
	assert.True(t, PortMappingEntry{Description: Description("web")}.IsOwned())
	assert.True(t, PortMappingEntry{Description: Description("")}.IsOwned())
	assert.False(t, PortMappingEntry{Description: "Plex"}.IsOwned())
}

func TestRouteSourceIP_RejectsLoopback(t *testing.T) {
	ip, err := routeSourceIP("127.0.0.1")
	// Loopback is never a valid forwarding target.
	assert.Error(t, err)
	assert.Empty(t, ip)
}
