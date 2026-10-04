package upnp

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp/upnptest"
)

func TestNewClient_WithGateway(t *testing.T) {
	igd := upnptest.NewIGD(t)
	ctx := context.Background()

	client, err := NewClient(ctx, "192.168.1.50", igd.URL(), time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "192.168.1.50", client.InternalIP())

	err = client.ForwardPorts(ctx, []types.PortMapping{
		{ExternalPort: 443, InternalPort: 8443, Protocol: "tcp", Name: "web"},
		{ExternalPort: 53, InternalPort: 53, Protocol: "UDP"},
	})
	require.NoError(t, err)
	assert.Equal(t, []upnptest.Mapping{
		{ExternalPort: 443, InternalPort: 8443, Protocol: "TCP", InternalIP: "192.168.1.50", Description: "Gangplank UPnP: web", Lease: 3600},
		{ExternalPort: 53, InternalPort: 53, Protocol: "UDP", InternalIP: "192.168.1.50", Description: "Gangplank UPnP", Lease: 3600},
	}, igd.Mappings())

	entries, err := client.ListPortMappings(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []PortMappingEntry{
		{ExternalPort: 443, InternalPort: 8443, Protocol: "TCP", InternalIP: "192.168.1.50", Description: "Gangplank UPnP: web", LeaseDuration: 3600, Enabled: true},
		{ExternalPort: 53, InternalPort: 53, Protocol: "UDP", InternalIP: "192.168.1.50", Description: "Gangplank UPnP", LeaseDuration: 3600, Enabled: true},
	}, entries)

	externalIP, err := client.GetExternalIP(ctx)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.7", externalIP)

	require.NoError(t, client.DeletePortMapping(ctx, 53, "udp"))
	assert.Len(t, igd.Mappings(), 1)

	err = client.DeletePortMapping(ctx, 53, "UDP")
	assert.True(t, hasErrorCode(err, errCodeNoSuchEntryInArray), "unexpected error: %v", err)
}

func TestNewClient_PermanentLeasesOnly(t *testing.T) {
	igd := upnptest.NewIGD(t)
	igd.PermanentOnly = true

	client, err := NewClient(context.Background(), "192.168.1.50", igd.URL(), time.Hour)
	require.NoError(t, err)

	require.NoError(t, client.ForwardPorts(context.Background(), []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}))
	require.Len(t, igd.Mappings(), 1)
	assert.Equal(t, 0, igd.Mappings()[0].Lease)
}

func TestNewClient_EmptyGatewayList(t *testing.T) {
	igd := upnptest.NewIGD(t)

	client, err := NewClient(context.Background(), "192.168.1.50", igd.URL(), time.Hour)
	require.NoError(t, err)

	entries, err := client.ListPortMappings(context.Background())
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestNewClient_InvalidGateway(t *testing.T) {
	tests := []struct {
		name    string
		gateway string
		wantErr string
	}{
		{name: "Missing scheme", gateway: "192.168.1.1", wantErr: "expected the IGD description URL"},
		{name: "Malformed URL", gateway: "http://[::1", wantErr: "invalid gateway URL"},
		{name: "Unreachable", gateway: "http://127.0.0.1:1/rootDesc.xml", wantErr: "no supported UPnP service found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := NewClient(ctx, "192.168.1.50", tt.gateway, time.Hour)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNewClient_DetectsLocalIP(t *testing.T) {
	igd := upnptest.NewIGD(t)

	client, err := NewClient(context.Background(), "", igd.URL(), time.Hour)
	if err != nil {
		// Only possible on a machine without any non-loopback IPv4 address.
		assert.Contains(t, err.Error(), "failed to determine local IP")
		return
	}
	ip := net.ParseIP(client.LocalIP)
	require.NotNil(t, ip)
	assert.False(t, ip.IsLoopback())
}

func TestGetLocalIP(t *testing.T) {
	loopbackGateway, _ := url.Parse("http://127.0.0.1:5000/rootDesc.xml")

	t.Run("Falls back to the discovery address", func(t *testing.T) {
		ip, err := getLocalIP(loopbackGateway, net.ParseIP("192.168.1.9"))
		require.NoError(t, err)
		assert.Equal(t, "192.168.1.9", ip)
	})

	t.Run("Ignores unusable discovery addresses", func(t *testing.T) {
		want, wantErr := firstInterfaceIP()
		for _, discovered := range []net.IP{nil, net.ParseIP("127.0.0.1"), net.IPv4zero, net.ParseIP("fe80::1")} {
			ip, err := getLocalIP(nil, discovered)
			assert.Equal(t, want, ip)
			assert.Equal(t, wantErr, err)
		}
	})
}

func TestNewDummyClient(t *testing.T) {
	client := NewDummyClient(time.Minute)
	assert.Equal(t, "192.168.1.100", client.InternalIP())

	ip, err := client.GetExternalIP(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.1", ip)
}

func TestClient_ForwardPorts_LeaseClamp(t *testing.T) {
	conn := &DummyConnection{}
	client := NewClientWithConnection(conn, "192.168.1.100", -time.Minute)

	require.NoError(t, client.ForwardPorts(context.Background(), []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}))

	// A negative TTL must not wrap around to a huge lease.
	assert.Len(t, conn.Forwarded, 1)
}

func TestClient_ForwardPorts_RespectsCancelledContext(t *testing.T) {
	igd := upnptest.NewIGD(t)
	client, err := NewClient(context.Background(), "192.168.1.50", igd.URL(), time.Hour)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = client.ForwardPorts(ctx, []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}})
	// goupnp formats errors with %v, so the cause is only visible in the message.
	require.Error(t, err)
	assert.Contains(t, err.Error(), context.Canceled.Error())
	assert.Empty(t, igd.Mappings())
}
