package gangplank

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/portmap"
	"github.com/IonBazan/gangplank/internal/providers"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/IonBazan/gangplank/internal/upnp/upnptest"
)

type MockPortProvider struct {
	Ports []portmap.Mapping
	Err   error
}

func (m *MockPortProvider) GetPortMappings(ctx context.Context) ([]portmap.Mapping, error) {
	return m.Ports, m.Err
}

type MockEventPortProvider struct {
	AddCh    chan portmap.Mapping
	DeleteCh chan portmap.Mapping
}

func (m *MockEventPortProvider) Listen(ctx context.Context, events providers.PortEventChannels) {
	for {
		select {
		case <-ctx.Done():
			return
		case p := <-m.AddCh:
			events.Add <- p
		case p := <-m.DeleteCh:
			if events.Delete != nil {
				events.Delete <- p
			}
		}
	}
}

func newClient(conn *upnptest.Connection) *upnp.Client {
	return upnp.NewClientWithConnection(conn, "192.168.1.100", upnp.DefaultLeaseDuration)
}

func TestManager_GetPortMappings(t *testing.T) {
	web := portmap.Mapping{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}
	db := portmap.Mapping{ExternalPort: 5432, InternalPort: 5432, Protocol: "TCP", Name: "db"}

	tests := []struct {
		name          string
		portProviders []providers.PortProvider
		wantPorts     []portmap.Mapping
		wantErr       bool
	}{
		{
			name: "Multiple PortProviders",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Ports: []portmap.Mapping{web}},
				&MockPortProvider{Ports: []portmap.Mapping{db}},
			},
			wantPorts: []portmap.Mapping{web, db},
		},
		{
			name:          "Empty PortProviders",
			portProviders: []providers.PortProvider{},
			wantPorts:     []portmap.Mapping{},
		},
		{
			name: "Provider error keeps other providers' mappings",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Ports: []portmap.Mapping{web}},
				&MockPortProvider{Err: errors.New("docker unavailable")},
			},
			wantPorts: []portmap.Mapping{web},
			wantErr:   true,
		},
		{
			name: "Conflicting mappings keep the first",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Ports: []portmap.Mapping{web}},
				&MockPortProvider{Ports: []portmap.Mapping{{ExternalPort: 8080, InternalPort: 8080, Protocol: "tcp", Name: "other"}}},
			},
			wantPorts: []portmap.Mapping{web},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Manager{portProviders: tt.portProviders}
			ports, err := g.GetPortMappings(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantPorts, ports)
		})
	}
}

func TestManager_Sync(t *testing.T) {
	ports := []portmap.Mapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}}

	t.Run("No UPnP client", func(t *testing.T) {
		g := &Manager{}
		assert.NoError(t, g.Sync(context.Background(), ports, false))
	})

	t.Run("Forward ports successfully", func(t *testing.T) {
		conn := &upnptest.Connection{}
		g := &Manager{}
		g.SetGateway(newClient(conn))

		assert.NoError(t, g.Sync(context.Background(), ports, false))
		forwarded, _ := conn.Snapshot()
		assert.Equal(t, []portmap.Mapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: web"}}, forwarded)
	})

	t.Run("Forward with error", func(t *testing.T) {
		conn := &upnptest.Connection{ForwardErr: errors.New("forward error")}
		g := &Manager{}
		g.SetGateway(newClient(conn))

		assert.ErrorIs(t, g.Sync(context.Background(), ports, false), conn.ForwardErr)
	})
}

func TestManager_SyncPrunesStaleMappings(t *testing.T) {
	conn := &upnptest.Connection{Existing: []upnptest.Mapping{
		{ExternalPort: 80, Protocol: "TCP", InternalIP: "192.168.1.100", Description: "Gangplank UPnP: web"},
		{ExternalPort: 81, Protocol: "TCP", InternalIP: "192.168.1.100", Description: "Gangplank UPnP: removed"},
		{ExternalPort: 82, Protocol: "TCP", InternalIP: "192.168.1.200", Description: "Gangplank UPnP: other host"},
		{ExternalPort: 83, Protocol: "UDP", InternalIP: "192.168.1.100", Description: "Plex"},
	}}
	g := &Manager{}
	g.SetGateway(newClient(conn))

	desired := []portmap.Mapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", Name: "web"}}

	require.NoError(t, g.Sync(context.Background(), desired, false))
	_, deleted := conn.Snapshot()
	assert.Empty(t, deleted, "nothing is pruned unless asked")

	require.NoError(t, g.Sync(context.Background(), desired, true))
	_, deleted = conn.Snapshot()
	assert.Equal(t, []upnptest.Deleted{{ExtPort: 81, Protocol: "TCP"}}, deleted)
}

func TestManager_Cleanup(t *testing.T) {
	conn := &upnptest.Connection{}
	g := &Manager{}
	g.SetGateway(newClient(conn))

	require.NoError(t, g.Sync(context.Background(), []portmap.Mapping{{ExternalPort: 80, InternalPort: 80, Protocol: "tcp", Name: "web"}}, false))
	require.NoError(t, g.Cleanup(context.Background()))

	_, deleted := conn.Snapshot()
	assert.Equal(t, []upnptest.Deleted{{ExtPort: 80, Protocol: "TCP"}}, deleted)
}

func TestManager_PollAndForward(t *testing.T) {
	web := portmap.Mapping{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}
	minecraft := portmap.Mapping{ExternalPort: 25565, InternalPort: 25565, Protocol: "TCP", Name: "minecraft"}

	tests := []struct {
		name        string
		withUPnP    bool
		cleanup     bool
		addEvents   []portmap.Mapping
		delEvents   []portmap.Mapping
		wantAdded   []portmap.Mapping
		wantDeleted []upnptest.Deleted
	}{
		{
			name:      "Poll without UPnP",
			addEvents: []portmap.Mapping{web},
		},
		{
			name:      "Poll and forward",
			withUPnP:  true,
			addEvents: []portmap.Mapping{web},
			wantAdded: []portmap.Mapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: web"}},
		},
		{
			name:        "Poll with cleanup",
			withUPnP:    true,
			cleanup:     true,
			delEvents:   []portmap.Mapping{minecraft},
			wantDeleted: []upnptest.Deleted{{ExtPort: 25565, Protocol: "TCP"}},
		},
		{
			name:      "Poll without cleanup ignores stops",
			withUPnP:  true,
			delEvents: []portmap.Mapping{minecraft},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &upnptest.Connection{}
			g := &Manager{}
			if tt.withUPnP {
				g.SetGateway(newClient(conn))
			}

			provider := &MockEventPortProvider{
				AddCh:    make(chan portmap.Mapping, len(tt.addEvents)),
				DeleteCh: make(chan portmap.Mapping, len(tt.delEvents)),
			}
			for _, p := range tt.addEvents {
				provider.AddCh <- p
			}
			for _, p := range tt.delEvents {
				provider.DeleteCh <- p
			}
			g.eventProviders = []providers.EventPortProvider{provider}

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				g.PollAndForward(ctx, tt.cleanup)
				close(done)
			}()

			assert.Eventually(t, func() bool {
				forwarded, deleted := conn.Snapshot()
				return len(forwarded) == len(tt.wantAdded) && len(deleted) == len(tt.wantDeleted) &&
					len(provider.AddCh) == 0 && len(provider.DeleteCh) == 0
			}, waitTimeout, 5*time.Millisecond)

			cancel()
			select {
			case <-done:
			case <-time.After(waitTimeout):
				t.Fatal("PollAndForward did not return after cancel")
			}

			forwarded, deleted := conn.Snapshot()
			assert.Equal(t, tt.wantAdded, forwarded)
			assert.Equal(t, tt.wantDeleted, deleted)
		})
	}
}

type failingForwarder struct {
	*upnp.Client
	listErr error
}

func (f failingForwarder) ListPortMappings(ctx context.Context) ([]upnp.PortMappingEntry, error) {
	return nil, f.listErr
}

func TestNewGangplank(t *testing.T) {
	g := NewManager(&config.Config{}, nil)

	require.Len(t, g.portProviders, 2)
	assert.IsType(t, &providers.ConfigPortProvider{}, g.portProviders[0])
	assert.IsType(t, &providers.DockerPortProvider{}, g.portProviders[1])
	require.Len(t, g.eventProviders, 1)
	assert.IsType(t, &providers.DockerEventPortProvider{}, g.eventProviders[0])

	assert.False(t, g.HasGateway())
	g.SetGateway(newClient(&upnptest.Connection{}))
	assert.True(t, g.HasGateway())
	g.SetGateway(nil)
	assert.False(t, g.HasGateway())
}

func TestManager_WithoutForwarder(t *testing.T) {
	g := &Manager{}
	ports := []portmap.Mapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}

	assert.NoError(t, g.Sync(context.Background(), ports, true))
	assert.NoError(t, g.Cleanup(context.Background()))
	g.delete(context.Background(), ports[0])
}

func TestManager_SyncReportsPruneErrors(t *testing.T) {
	desired := []portmap.Mapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}

	t.Run("List fails", func(t *testing.T) {
		g := &Manager{}
		g.SetGateway(failingForwarder{Client: newClient(&upnptest.Connection{}), listErr: errors.New("list failed")})

		err := g.Sync(context.Background(), desired, true)
		assert.ErrorContains(t, err, "failed to list gateway mappings")
	})

	t.Run("Delete fails", func(t *testing.T) {
		conn := &upnptest.Connection{
			DeleteErr: errors.New("delete failed"),
			Existing: []upnptest.Mapping{
				{ExternalPort: 81, Protocol: "tcp", InternalIP: "192.168.1.100", Description: "Gangplank UPnP: old"},
			},
		}
		g := &Manager{}
		g.SetGateway(newClient(conn))

		err := g.Sync(context.Background(), desired, true)
		assert.ErrorContains(t, err, "prune 81/TCP")
	})
}

func TestManager_SyncReplacesForwardedSet(t *testing.T) {
	conn := &upnptest.Connection{}
	g := &Manager{}
	g.SetGateway(newClient(conn))
	ctx := context.Background()

	require.NoError(t, g.Sync(ctx, []portmap.Mapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}, false))
	require.NoError(t, g.Sync(ctx, []portmap.Mapping{{ExternalPort: 443, InternalPort: 443, Protocol: "TCP"}}, false))
	require.NoError(t, g.Cleanup(ctx))

	// Only the latest desired set is cleaned up.
	_, deleted := conn.Snapshot()
	assert.Equal(t, []upnptest.Deleted{{ExtPort: 443, Protocol: "TCP"}}, deleted)
}

func TestManager_CleanupReportsErrors(t *testing.T) {
	conn := &upnptest.Connection{}
	g := &Manager{}
	g.SetGateway(newClient(conn))
	require.NoError(t, g.Sync(context.Background(), []portmap.Mapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}, false))

	conn.DeleteErr = errors.New("delete failed")
	assert.ErrorContains(t, g.Cleanup(context.Background()), "delete 80/TCP")
}

func TestManager_PollAndForwardSurvivesErrors(t *testing.T) {
	conn := &upnptest.Connection{ForwardErr: errors.New("forward failed"), DeleteErr: errors.New("delete failed")}
	g := &Manager{}
	g.SetGateway(newClient(conn))

	provider := &MockEventPortProvider{AddCh: make(chan portmap.Mapping, 2), DeleteCh: make(chan portmap.Mapping, 1)}
	provider.AddCh <- portmap.Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}
	provider.DeleteCh <- portmap.Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}
	provider.AddCh <- portmap.Mapping{ExternalPort: 81, InternalPort: 81, Protocol: "TCP"}
	g.eventProviders = []providers.EventPortProvider{provider}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		g.PollAndForward(ctx, true)
		close(done)
	}()

	// Errors are logged and the loop keeps consuming events.
	assert.Eventually(t, func() bool {
		return len(provider.AddCh) == 0 && len(provider.DeleteCh) == 0
	}, waitTimeout, 5*time.Millisecond)

	cancel()
	<-done
}

func TestManager_EventsRespectPortOwnership(t *testing.T) {
	conn := &upnptest.Connection{}
	g := &Manager{}
	g.SetGateway(newClient(conn))
	ctx := context.Background()

	fromConfig := portmap.Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", Name: "nginx"}
	require.NoError(t, g.Sync(ctx, []portmap.Mapping{fromConfig}, false))

	// A container asking for the same port neither takes it over nor closes it.
	g.add(ctx, portmap.Mapping{ExternalPort: 80, InternalPort: 8080, Protocol: "tcp", Name: "web"})
	g.delete(ctx, portmap.Mapping{ExternalPort: 80, InternalPort: 8080, Protocol: "tcp", Name: "web"})

	forwarded, deleted := conn.Snapshot()
	assert.Len(t, forwarded, 1, "only the initial sync forwarded the port")
	assert.Empty(t, deleted)

	// The owner itself can still close it.
	g.delete(ctx, fromConfig)
	_, deleted = conn.Snapshot()
	assert.Equal(t, []upnptest.Deleted{{ExtPort: 80, Protocol: "TCP"}}, deleted)
}

func TestManager_EventsForFreePorts(t *testing.T) {
	conn := &upnptest.Connection{}
	g := &Manager{}
	g.SetGateway(newClient(conn))
	ctx := context.Background()
	game := portmap.Mapping{ExternalPort: 25565, InternalPort: 25565, Protocol: "TCP", Name: "game"}

	g.add(ctx, game)
	g.add(ctx, game) // a restarted container claims its own port again
	g.delete(ctx, game)
	g.delete(ctx, game) // stop and die both arrive

	forwarded, deleted := conn.Snapshot()
	assert.Len(t, forwarded, 2)
	assert.Len(t, deleted, 2, "deleting an unknown port is still passed to the gateway")
}
