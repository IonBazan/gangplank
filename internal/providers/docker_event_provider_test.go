package providers

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/portmap"
)

// waitTimeout is only reached when a test fails, so it can be generous for slow CI runners.
const waitTimeout = 5 * time.Second

type eventStream struct {
	msgs chan events.Message
	errs chan error
}

type MockEventClient struct {
	mu        sync.Mutex
	streams   []eventStream
	current   eventStream
	subscribe chan struct{}
	listed    chan struct{}
	Running   []container.Summary
	Inspect   map[string]container.InspectResponse
	// ListErrs are returned by successive ContainerList calls before it succeeds.
	ListErrs []error
}

func newMockEventClient(streams int) *MockEventClient {
	m := &MockEventClient{
		subscribe: make(chan struct{}, streams+1),
		listed:    make(chan struct{}, 10),
		Inspect:   map[string]container.InspectResponse{},
	}
	for range streams {
		m.streams = append(m.streams, eventStream{make(chan events.Message, 10), make(chan error, 1)})
	}
	return m
}

func (m *MockEventClient) Events(ctx context.Context, options client.EventsListOptions) client.EventsResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	defer func() { m.subscribe <- struct{}{} }()
	if len(m.streams) == 0 {
		// Never deliver anything once the scripted streams are used up.
		m.current = eventStream{make(chan events.Message, 10), make(chan error, 1)}
	} else {
		m.current = m.streams[0]
		m.streams = m.streams[1:]
	}
	return client.EventsResult{Messages: m.current.msgs, Err: m.current.errs}
}

func (m *MockEventClient) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	defer func() {
		select {
		case m.listed <- struct{}{}:
		default:
		}
	}()
	if len(m.ListErrs) > 0 {
		err := m.ListErrs[0]
		m.ListErrs = m.ListErrs[1:]
		return client.ContainerListResult{}, err
	}
	return client.ContainerListResult{Items: append([]container.Summary(nil), m.Running...)}, nil
}

func (m *MockEventClient) ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if info, ok := m.Inspect[containerID]; ok {
		return client.ContainerInspectResult{Container: info}, nil
	}
	return client.ContainerInspectResult{}, assert.AnError
}

func inspectResponse(id, name string, labels map[string]string, ports network.PortMap) container.InspectResponse {
	return container.InspectResponse{
		ID:              id,
		Name:            "/" + name,
		NetworkSettings: &container.NetworkSettings{Ports: ports},
		Config:          &container.Config{Labels: labels},
	}
}

func collect(ch <-chan portmap.Mapping, n int) []portmap.Mapping {
	var got []portmap.Mapping
	timeout := time.After(waitTimeout)
	for len(got) < n {
		select {
		case m := <-ch:
			got = append(got, m)
		case <-timeout:
			return got
		}
	}
	return got
}

func assertNoMore(t *testing.T, ch <-chan portmap.Mapping) {
	t.Helper()
	select {
	case m := <-ch:
		t.Fatalf("unexpected mapping %+v", m)
	case <-time.After(50 * time.Millisecond):
	}
}

// waitListed waits until the provider has listed the running containers, which it does after subscribing.
func waitListed(t *testing.T, m *MockEventClient) {
	t.Helper()
	select {
	case <-m.listed:
	case <-time.After(waitTimeout):
		t.Fatal("provider did not list containers")
	}
}

func waitSubscribed(t *testing.T, m *MockEventClient) {
	t.Helper()
	select {
	case <-m.subscribe:
	case <-time.After(waitTimeout):
		t.Fatal("provider did not subscribe to events")
	}
}

func TestDockerEventPortProvider_Listen(t *testing.T) {
	tests := []struct {
		name       string
		inspect    container.InspectResponse
		events     []events.Action
		wantAdd    []portmap.Mapping
		wantDelete []portmap.Mapping
	}{
		{
			name: "Nginx start with published ports",
			inspect: inspectResponse("nginx1234567890", "nginx", map[string]string{labelForward: "published"},
				network.PortMap{network.MustParsePort("80/tcp"): {{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: "8080"}, {HostIP: netip.MustParseAddr("::"), HostPort: "8080"}}}),
			events:  []events.Action{events.ActionStart},
			wantAdd: []portmap.Mapping{{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "nginx"}},
		},
		{
			name: "Redis start, stop and die deletes once",
			inspect: inspectResponse("redis4567890123", "redis", map[string]string{labelForward: "6379:6379/tcp"},
				network.PortMap{network.MustParsePort("6379/tcp"): {{HostPort: "6379"}}}),
			events:     []events.Action{events.ActionStart, events.ActionDie, events.ActionStop},
			wantAdd:    []portmap.Mapping{{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"}},
			wantDelete: []portmap.Mapping{{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"}},
		},
		{
			name: "Postgres start with container-referenced label",
			inspect: inspectResponse("pg7890123456789", "postgres", map[string]string{labelForwardContainer: "5432/tcp"},
				network.PortMap{network.MustParsePort("5432/tcp"): {{HostPort: "5433"}}}),
			events:  []events.Action{events.ActionStart},
			wantAdd: []portmap.Mapping{{ExternalPort: 5432, InternalPort: 5433, Protocol: "TCP", Name: "postgres"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockEventClient(1)
			mockClient.Inspect[tt.inspect.ID] = tt.inspect
			provider := NewDockerEventPortProvider(mockClient)

			addCh := make(chan portmap.Mapping, 10)
			deleteCh := make(chan portmap.Mapping, 10)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go provider.Listen(ctx, PortEventChannels{Add: addCh, Delete: deleteCh})
			waitSubscribed(t, mockClient)

			for _, action := range tt.events {
				mockClient.streamsSend(events.Message{Action: action, Actor: events.Actor{ID: tt.inspect.ID}})
			}

			assert.ElementsMatch(t, tt.wantAdd, collect(addCh, len(tt.wantAdd)))
			assert.ElementsMatch(t, tt.wantDelete, collect(deleteCh, len(tt.wantDelete)))
			assertNoMore(t, addCh)
			assertNoMore(t, deleteCh)
		})
	}
}

func (m *MockEventClient) streamsSend(msg events.Message) {
	m.mu.Lock()
	s := m.current
	m.mu.Unlock()
	s.msgs <- msg
}

func TestDockerEventPortProvider_TracksContainersRunningAtStartup(t *testing.T) {
	mockClient := newMockEventClient(1)
	mockClient.Running = []container.Summary{{
		ID:     "web123456789012",
		Names:  []string{"/web"},
		Labels: map[string]string{labelForward: "published"},
		Ports:  []container.PortSummary{{PublicPort: 80, PrivatePort: 80, Type: "tcp"}},
	}}
	provider := NewDockerEventPortProvider(mockClient)

	addCh := make(chan portmap.Mapping, 10)
	deleteCh := make(chan portmap.Mapping, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go provider.Listen(ctx, PortEventChannels{Add: addCh, Delete: deleteCh})
	waitSubscribed(t, mockClient)

	// Initial sync must not re-announce containers the daemon already forwarded.
	assertNoMore(t, addCh)

	mockClient.streamsSend(events.Message{Action: events.ActionStop, Actor: events.Actor{ID: "web123456789012"}})
	assert.Equal(t, []portmap.Mapping{{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", Name: "web"}}, collect(deleteCh, 1))
}

func TestDockerEventPortProvider_ReconnectsAndResyncs(t *testing.T) {
	mockClient := newMockEventClient(2)
	mockClient.Running = []container.Summary{{
		ID:     "old123456789012",
		Names:  []string{"/old"},
		Labels: map[string]string{labelForward: "1000"},
	}}
	provider := NewDockerEventPortProvider(mockClient)
	provider.retryDelay = time.Millisecond

	addCh := make(chan portmap.Mapping, 10)
	deleteCh := make(chan portmap.Mapping, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go provider.Listen(ctx, PortEventChannels{Add: addCh, Delete: deleteCh})
	waitSubscribed(t, mockClient)
	waitListed(t, mockClient)

	// While disconnected, "old" stops and "new" starts.
	mockClient.mu.Lock()
	mockClient.Running = []container.Summary{{
		ID:     "new123456789012",
		Names:  []string{"/new"},
		Labels: map[string]string{labelForward: "2000"},
	}}
	mockClient.current.errs <- errors.New("daemon restarted")
	mockClient.mu.Unlock()

	waitSubscribed(t, mockClient)
	require.Equal(t, []portmap.Mapping{{ExternalPort: 2000, InternalPort: 2000, Protocol: "TCP", Name: "new"}}, collect(addCh, 1))
	require.Equal(t, []portmap.Mapping{{ExternalPort: 1000, InternalPort: 1000, Protocol: "TCP", Name: "old"}}, collect(deleteCh, 1))
}

func TestDockerEventPortProvider_NilDeleteChannel(t *testing.T) {
	mockClient := newMockEventClient(1)
	mockClient.Inspect["svc12345678901"] = inspectResponse("svc12345678901", "svc", map[string]string{labelForward: "81"}, nil)
	provider := NewDockerEventPortProvider(mockClient)

	addCh := make(chan portmap.Mapping, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		provider.Listen(ctx, PortEventChannels{Add: addCh})
		close(done)
	}()
	waitSubscribed(t, mockClient)

	mockClient.streamsSend(events.Message{Action: events.ActionStart, Actor: events.Actor{ID: "svc12345678901"}})
	mockClient.streamsSend(events.Message{Action: events.ActionStop, Actor: events.Actor{ID: "svc12345678901"}})
	mockClient.streamsSend(events.Message{Action: events.ActionStart, Actor: events.Actor{ID: "svc12345678901"}})
	// Stop with no delete channel must not block the event loop.
	assert.Len(t, collect(addCh, 2), 2)

	cancel()
	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("Listen did not return after cancel")
	}
}

func TestDockerEventPortProvider_RetriesWhenDockerIsUnavailable(t *testing.T) {
	mockClient := newMockEventClient(0)
	mockClient.ListErrs = []error{errors.New("docker down"), errors.New("still down")}
	mockClient.Inspect["app12345678901"] = inspectResponse("app12345678901", "app", map[string]string{labelForward: "8080"}, nil)
	provider := NewDockerEventPortProvider(mockClient)
	provider.retryDelay = time.Millisecond

	addCh := make(chan portmap.Mapping, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go provider.Listen(ctx, PortEventChannels{Add: addCh})

	// Two failed attempts, then a working connection.
	waitSubscribed(t, mockClient)
	waitSubscribed(t, mockClient)
	waitSubscribed(t, mockClient)

	mockClient.streamsSend(events.Message{Action: events.ActionStart, Actor: events.Actor{ID: "app12345678901"}})
	assert.Equal(t, []portmap.Mapping{{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "app"}}, collect(addCh, 1))
}

func TestDockerEventPortProvider_ReconnectsWhenStreamCloses(t *testing.T) {
	mockClient := newMockEventClient(1)
	provider := NewDockerEventPortProvider(mockClient)
	provider.retryDelay = time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go provider.Listen(ctx, PortEventChannels{})

	waitSubscribed(t, mockClient)
	mockClient.mu.Lock()
	close(mockClient.current.msgs)
	mockClient.mu.Unlock()
	waitSubscribed(t, mockClient)
}

func TestDockerEventPortProvider_IgnoresUninspectableContainers(t *testing.T) {
	mockClient := newMockEventClient(1)
	provider := NewDockerEventPortProvider(mockClient)

	addCh := make(chan portmap.Mapping, 10)
	deleteCh := make(chan portmap.Mapping, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go provider.Listen(ctx, PortEventChannels{Add: addCh, Delete: deleteCh})
	waitSubscribed(t, mockClient)

	// Unknown container: inspect fails, and its stop has nothing to delete.
	mockClient.streamsSend(events.Message{Action: events.ActionStart, Actor: events.Actor{ID: "gone"}})
	mockClient.streamsSend(events.Message{Action: events.ActionStop, Actor: events.Actor{ID: "gone"}})
	// Other actions are ignored.
	mockClient.streamsSend(events.Message{Action: events.ActionRestart, Actor: events.Actor{ID: "gone"}})

	assertNoMore(t, addCh)
	assertNoMore(t, deleteCh)
}

func TestSend_StopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		// Unbuffered channel without a reader would block forever.
		send(ctx, make(chan portmap.Mapping), []portmap.Mapping{{ExternalPort: 80}})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(waitTimeout):
		t.Fatal("send blocked after cancel")
	}
}

func TestPortsFromInspect(t *testing.T) {
	assert.Nil(t, portsFromInspect(container.InspectResponse{}))

	info := inspectResponse("id", "name", nil, network.PortMap{
		network.MustParsePort("80/tcp"): {
			{HostIP: netip.MustParseAddr("0.0.0.0"), HostPort: "8080"},
			{HostIP: netip.MustParseAddr("::1"), HostPort: "8081"},
			{HostPort: ""},
			{HostPort: "abc"},
			{HostPort: "70000"},
		},
		network.MustParsePort("53/udp"): nil,
	})

	assert.Equal(t, []container.PortSummary{
		{IP: netip.MustParseAddr("0.0.0.0"), PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
		{IP: netip.MustParseAddr("::1"), PrivatePort: 80, PublicPort: 8081, Type: "tcp"},
	}, portsFromInspect(info))
}
