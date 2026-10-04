package gangplank

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/portmap"
	"github.com/IonBazan/gangplank/internal/providers"
	"github.com/IonBazan/gangplank/internal/upnp"
)

// waitTimeout is only reached when a test fails, so it can be generous for slow CI runners.
const waitTimeout = 5 * time.Second

func startDaemon(t *testing.T, d *Daemon) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()

	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(waitTimeout):
			t.Fatal("daemon did not stop")
		}
	}
}

func staticManager(ports ...portmap.Mapping) *Manager {
	return &Manager{PortProviders: []providers.PortProvider{&MockPortProvider{Ports: ports}}}
}

func connectTo(conn *upnp.DummyConnection) func(context.Context) (Gateway, error) {
	return func(context.Context) (Gateway, error) { return newClient(conn), nil }
}

func TestDaemon_RefreshesAndCleansUpOnExit(t *testing.T) {
	conn := &upnp.DummyConnection{}
	web := portmap.Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", Name: "web"}
	d := NewDaemon(staticManager(web), connectTo(conn), DaemonOptions{RefreshInterval: 10 * time.Millisecond, CleanupOnExit: true})

	stop := startDaemon(t, d)
	assert.Eventually(t, func() bool {
		forwarded, _ := conn.Snapshot()
		return len(forwarded) >= 2
	}, waitTimeout, 5*time.Millisecond, "mappings are renewed on every refresh")
	stop()

	_, deleted := conn.Snapshot()
	assert.Equal(t, []upnp.DeletedMapping{{ExtPort: 80, Protocol: "TCP"}}, deleted)
}

func TestDaemon_KeepsMappingsOnExitByDefault(t *testing.T) {
	conn := &upnp.DummyConnection{}
	d := NewDaemon(staticManager(portmap.Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}), connectTo(conn), DaemonOptions{RefreshInterval: time.Hour})

	stop := startDaemon(t, d)
	assert.Eventually(t, func() bool {
		forwarded, _ := conn.Snapshot()
		return len(forwarded) == 1
	}, waitTimeout, 5*time.Millisecond)
	stop()

	_, deleted := conn.Snapshot()
	assert.Empty(t, deleted)
}

func TestDaemon_RetriesGatewayUntilAvailable(t *testing.T) {
	conn := &upnp.DummyConnection{}
	var attempts atomic.Int32
	connect := func(ctx context.Context) (Gateway, error) {
		if attempts.Add(1) < 3 {
			return nil, errors.New("router is booting")
		}
		return newClient(conn), nil
	}
	d := NewDaemon(staticManager(portmap.Mapping{ExternalPort: 80, InternalPort: 80, Protocol: "TCP"}), connect, DaemonOptions{RefreshInterval: time.Hour})
	d.retryInterval = 5 * time.Millisecond

	stop := startDaemon(t, d)
	assert.Eventually(t, func() bool {
		forwarded, _ := conn.Snapshot()
		return len(forwarded) == 1
	}, waitTimeout, 5*time.Millisecond, "retries use the short interval, not the refresh interval")
	stop()
	assert.Equal(t, int32(3), attempts.Load())
}

func TestDaemon_PollsEvents(t *testing.T) {
	conn := &upnp.DummyConnection{}
	events := &MockEventPortProvider{AddCh: make(chan portmap.Mapping, 1), DeleteCh: make(chan portmap.Mapping, 1)}
	m := staticManager()
	m.EventPortProviders = []providers.EventPortProvider{events}
	d := NewDaemon(m, connectTo(conn), DaemonOptions{RefreshInterval: time.Hour, Poll: true, CleanupOnStop: true})

	stop := startDaemon(t, d)
	require.Eventually(t, m.HasGateway, waitTimeout, 5*time.Millisecond)

	events.AddCh <- portmap.Mapping{ExternalPort: 25565, InternalPort: 25565, Protocol: "TCP", Name: "game"}
	assert.Eventually(t, func() bool {
		forwarded, _ := conn.Snapshot()
		return len(forwarded) == 1
	}, waitTimeout, 5*time.Millisecond)

	events.DeleteCh <- portmap.Mapping{ExternalPort: 25565, InternalPort: 25565, Protocol: "TCP", Name: "game"}
	assert.Eventually(t, func() bool {
		_, deleted := conn.Snapshot()
		return len(deleted) == 1
	}, waitTimeout, 5*time.Millisecond)
	stop()
}
