package internal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/providers"
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
)

type MockPortProvider struct {
	Ports []types.PortMapping
	Err   error
}

func (m *MockPortProvider) GetPortMappings(ctx context.Context) ([]types.PortMapping, error) {
	return m.Ports, m.Err
}

type MockEventPortProvider struct {
	AddCh    chan types.PortMapping
	DeleteCh chan types.PortMapping
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

func newClient(conn *upnp.DummyConnection) *upnp.Client {
	return upnp.NewClientWithConnection(conn, "192.168.1.100", upnp.DefaultLeaseDuration)
}

func TestGangplank_GetPortMappings(t *testing.T) {
	web := types.PortMapping{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}
	db := types.PortMapping{ExternalPort: 5432, InternalPort: 5432, Protocol: "TCP", Name: "db"}

	tests := []struct {
		name          string
		portProviders []providers.PortProvider
		wantPorts     []types.PortMapping
		wantErr       bool
	}{
		{
			name: "Multiple PortProviders",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Ports: []types.PortMapping{web}},
				&MockPortProvider{Ports: []types.PortMapping{db}},
			},
			wantPorts: []types.PortMapping{web, db},
		},
		{
			name:          "Empty PortProviders",
			portProviders: []providers.PortProvider{},
			wantPorts:     []types.PortMapping{},
		},
		{
			name: "Provider error keeps other providers' mappings",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Ports: []types.PortMapping{web}},
				&MockPortProvider{Err: errors.New("docker unavailable")},
			},
			wantPorts: []types.PortMapping{web},
			wantErr:   true,
		},
		{
			name: "Conflicting mappings keep the first",
			portProviders: []providers.PortProvider{
				&MockPortProvider{Ports: []types.PortMapping{web}},
				&MockPortProvider{Ports: []types.PortMapping{{ExternalPort: 8080, InternalPort: 8080, Protocol: "tcp", Name: "other"}}},
			},
			wantPorts: []types.PortMapping{web},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Gangplank{PortProviders: tt.portProviders}
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

func TestGangplank_ForwardPorts(t *testing.T) {
	ports := []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}}

	t.Run("No UPnP client", func(t *testing.T) {
		g := &Gangplank{}
		assert.NoError(t, g.ForwardPorts(context.Background(), ports))
	})

	t.Run("Forward ports successfully", func(t *testing.T) {
		conn := &upnp.DummyConnection{}
		g := &Gangplank{}
		g.SetForwarder(newClient(conn))

		assert.NoError(t, g.ForwardPorts(context.Background(), ports))
		forwarded, _ := conn.Snapshot()
		assert.Equal(t, []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: web"}}, forwarded)
	})

	t.Run("Forward with error", func(t *testing.T) {
		conn := &upnp.DummyConnection{ForwardErr: errors.New("forward error")}
		g := &Gangplank{}
		g.SetForwarder(newClient(conn))

		assert.ErrorIs(t, g.ForwardPorts(context.Background(), ports), conn.ForwardErr)
	})
}

func TestGangplank_SyncPrunesStaleMappings(t *testing.T) {
	conn := &upnp.DummyConnection{Existing: []upnp.PortMappingEntry{
		{ExternalPort: 80, Protocol: "TCP", InternalIP: "192.168.1.100", Description: "Gangplank UPnP: web"},
		{ExternalPort: 81, Protocol: "TCP", InternalIP: "192.168.1.100", Description: "Gangplank UPnP: removed"},
		{ExternalPort: 82, Protocol: "TCP", InternalIP: "192.168.1.200", Description: "Gangplank UPnP: other host"},
		{ExternalPort: 83, Protocol: "UDP", InternalIP: "192.168.1.100", Description: "Plex"},
	}}
	g := &Gangplank{}
	g.SetForwarder(newClient(conn))

	desired := []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", Name: "web"}}

	require.NoError(t, g.Sync(context.Background(), desired, false))
	_, deleted := conn.Snapshot()
	assert.Empty(t, deleted, "nothing is pruned unless asked")

	require.NoError(t, g.Sync(context.Background(), desired, true))
	_, deleted = conn.Snapshot()
	assert.Equal(t, []upnp.DeletedMapping{{ExtPort: 81, Protocol: "TCP"}}, deleted)
}

func TestGangplank_Cleanup(t *testing.T) {
	conn := &upnp.DummyConnection{}
	g := &Gangplank{}
	g.SetForwarder(newClient(conn))

	require.NoError(t, g.Sync(context.Background(), []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "tcp", Name: "web"}}, false))
	require.NoError(t, g.Cleanup(context.Background()))

	_, deleted := conn.Snapshot()
	assert.Equal(t, []upnp.DeletedMapping{{ExtPort: 80, Protocol: "TCP"}}, deleted)
}

func TestGangplank_PollAndForward(t *testing.T) {
	web := types.PortMapping{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}
	minecraft := types.PortMapping{ExternalPort: 25565, InternalPort: 25565, Protocol: "TCP", Name: "minecraft"}

	tests := []struct {
		name        string
		withUPnP    bool
		cleanup     bool
		addEvents   []types.PortMapping
		delEvents   []types.PortMapping
		wantAdded   []types.PortMapping
		wantDeleted []upnp.DeletedMapping
	}{
		{
			name:      "Poll without UPnP",
			addEvents: []types.PortMapping{web},
		},
		{
			name:      "Poll and forward",
			withUPnP:  true,
			addEvents: []types.PortMapping{web},
			wantAdded: []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "Gangplank UPnP: web"}},
		},
		{
			name:        "Poll with cleanup",
			withUPnP:    true,
			cleanup:     true,
			delEvents:   []types.PortMapping{minecraft},
			wantDeleted: []upnp.DeletedMapping{{ExtPort: 25565, Protocol: "TCP"}},
		},
		{
			name:      "Poll without cleanup ignores stops",
			withUPnP:  true,
			delEvents: []types.PortMapping{minecraft},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &upnp.DummyConnection{}
			g := &Gangplank{}
			if tt.withUPnP {
				g.SetForwarder(newClient(conn))
			}

			provider := &MockEventPortProvider{
				AddCh:    make(chan types.PortMapping, len(tt.addEvents)),
				DeleteCh: make(chan types.PortMapping, len(tt.delEvents)),
			}
			for _, p := range tt.addEvents {
				provider.AddCh <- p
			}
			for _, p := range tt.delEvents {
				provider.DeleteCh <- p
			}
			g.EventPortProviders = []providers.EventPortProvider{provider}

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
			}, time.Second, 5*time.Millisecond)

			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
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
	g := NewGangplank(&config.Config{}, nil)

	require.Len(t, g.PortProviders, 2)
	assert.IsType(t, &providers.ConfigPortProvider{}, g.PortProviders[0])
	assert.IsType(t, &providers.DockerPortProvider{}, g.PortProviders[1])
	require.Len(t, g.EventPortProviders, 1)
	assert.IsType(t, &providers.DockerEventPortProvider{}, g.EventPortProviders[0])

	assert.False(t, g.HasForwarder())
	g.SetForwarder(newClient(&upnp.DummyConnection{}))
	assert.True(t, g.HasForwarder())
	g.SetForwarder(nil)
	assert.False(t, g.HasForwarder())
}

func TestGangplank_WithoutForwarder(t *testing.T) {
	g := &Gangplank{}
	ports := []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}

	assert.NoError(t, g.Sync(context.Background(), ports, true))
	assert.NoError(t, g.Cleanup(context.Background()))
	g.delete(context.Background(), ports[0])
}

func TestGangplank_SyncReportsPruneErrors(t *testing.T) {
	desired := []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}

	t.Run("List fails", func(t *testing.T) {
		g := &Gangplank{}
		g.SetForwarder(failingForwarder{Client: newClient(&upnp.DummyConnection{}), listErr: errors.New("list failed")})

		err := g.Sync(context.Background(), desired, true)
		assert.ErrorContains(t, err, "failed to list gateway mappings")
	})

	t.Run("Delete fails", func(t *testing.T) {
		conn := &upnp.DummyConnection{
			DeleteErr: errors.New("delete failed"),
			Existing: []upnp.PortMappingEntry{
				{ExternalPort: 81, Protocol: "tcp", InternalIP: "192.168.1.100", Description: "Gangplank UPnP: old"},
			},
		}
		g := &Gangplank{}
		g.SetForwarder(newClient(conn))

		err := g.Sync(context.Background(), desired, true)
		assert.ErrorContains(t, err, "prune 81/TCP")
	})
}

func TestGangplank_SyncReplacesForwardedSet(t *testing.T) {
	conn := &upnp.DummyConnection{}
	g := &Gangplank{}
	g.SetForwarder(newClient(conn))
	ctx := context.Background()

	require.NoError(t, g.Sync(ctx, []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}, false))
	require.NoError(t, g.Sync(ctx, []types.PortMapping{{ExternalPort: 443, InternalPort: 443, Protocol: "TCP"}}, false))
	require.NoError(t, g.Cleanup(ctx))

	// Only the latest desired set is cleaned up.
	_, deleted := conn.Snapshot()
	assert.Equal(t, []upnp.DeletedMapping{{ExtPort: 443, Protocol: "TCP"}}, deleted)
}

func TestGangplank_CleanupReportsErrors(t *testing.T) {
	conn := &upnp.DummyConnection{}
	g := &Gangplank{}
	g.SetForwarder(newClient(conn))
	require.NoError(t, g.ForwardPorts(context.Background(), []types.PortMapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}}))

	conn.DeleteErr = errors.New("delete failed")
	assert.ErrorContains(t, g.Cleanup(context.Background()), "delete 80/TCP")
}

func TestGangplank_PollAndForwardSurvivesErrors(t *testing.T) {
	conn := &upnp.DummyConnection{ForwardErr: errors.New("forward failed"), DeleteErr: errors.New("delete failed")}
	g := &Gangplank{}
	g.SetForwarder(newClient(conn))

	provider := &MockEventPortProvider{AddCh: make(chan types.PortMapping, 2), DeleteCh: make(chan types.PortMapping, 1)}
	provider.AddCh <- types.PortMapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}
	provider.DeleteCh <- types.PortMapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}
	provider.AddCh <- types.PortMapping{ExternalPort: 81, InternalPort: 81, Protocol: "TCP"}
	g.EventPortProviders = []providers.EventPortProvider{provider}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		g.PollAndForward(ctx, true)
		close(done)
	}()

	// Errors are logged and the loop keeps consuming events.
	assert.Eventually(t, func() bool {
		return len(provider.AddCh) == 0 && len(provider.DeleteCh) == 0
	}, time.Second, 5*time.Millisecond)

	cancel()
	<-done
}
