package fetchers

import (
	"context"
	"github.com/docker/go-connections/nat"
	"testing"
	"time"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/stretchr/testify/assert"
)

type MockEventClient struct {
	EventsChan chan events.Message
	ErrChan    chan error
	Inspect    map[string]container.InspectResponse
}

func (m *MockEventClient) Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error) {
	return m.EventsChan, m.ErrChan
}

func (m *MockEventClient) ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error) {
	if ctx == nil {
		return container.InspectResponse{}, assert.AnError
	}
	if info, ok := m.Inspect[containerID]; ok {
		return info, nil
	}
	return container.InspectResponse{}, assert.AnError
}

func TestDockerEventPortFetcher_Listen(t *testing.T) {
	tests := []struct {
		name       string
		containers map[string]container.InspectResponse
		events     []events.Message
		wantAdd    []types.PortMapping
		wantDelete []types.PortMapping
	}{
		{
			name: "Nginx start with published ports",
			containers: map[string]container.InspectResponse{
				"nginx1234567890": {
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:   "nginx1234567890",
						Name: "/nginx",
					},
					NetworkSettings: &container.NetworkSettings{
						NetworkSettingsBase: container.NetworkSettingsBase{
							Ports: map[nat.Port][]nat.PortBinding{
								"80/tcp": {{HostPort: "8080"}},
							},
						},
					},
					Config: &container.Config{
						Labels: map[string]string{
							labelForward: "published",
						},
					},
				},
			},
			events: []events.Message{
				{Action: "start", Actor: events.Actor{ID: "nginx1234567890"}},
			},
			wantAdd: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
			wantDelete: []types.PortMapping{},
		},
		{
			name: "Redis start and stop with host-referenced label",
			containers: map[string]container.InspectResponse{
				"redis4567890123": {
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:   "redis4567890123",
						Name: "/redis",
					},
					NetworkSettings: &container.NetworkSettings{
						NetworkSettingsBase: container.NetworkSettingsBase{
							Ports: map[nat.Port][]nat.PortBinding{
								"6379/tcp": {{HostPort: "6379"}},
							},
						},
					},
					Config: &container.Config{
						Labels: map[string]string{
							labelForward: "6379:6379/tcp",
						},
					},
				},
			},
			events: []events.Message{
				{Action: "start", Actor: events.Actor{ID: "redis4567890123"}},
				{Action: "stop", Actor: events.Actor{ID: "redis4567890123"}},
			},
			wantAdd: []types.PortMapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
			},
			wantDelete: []types.PortMapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
			},
		},
		{
			name: "Postgres start with container-referenced label",
			containers: map[string]container.InspectResponse{
				"pg7890123456789": {
					ContainerJSONBase: &container.ContainerJSONBase{
						ID:   "pg7890123456789",
						Name: "/postgres",
					},
					NetworkSettings: &container.NetworkSettings{
						NetworkSettingsBase: container.NetworkSettingsBase{
							Ports: map[nat.Port][]nat.PortBinding{
								"5432/tcp": {{HostPort: "5433"}},
							},
						},
					},
					Config: &container.Config{
						Labels: map[string]string{
							labelForwardContainer: "5432/tcp",
						},
					},
				},
			},
			events: []events.Message{
				{Action: "start", Actor: events.Actor{ID: "pg7890123456789"}},
			},
			wantAdd: []types.PortMapping{
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "TCP", Name: "postgres"},
			},
			wantDelete: []types.PortMapping{},
		},
		{
			name:       "No events",
			containers: map[string]container.InspectResponse{},
			events:     []events.Message{},
			wantAdd:    []types.PortMapping{},
			wantDelete: []types.PortMapping{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventsChan := make(chan events.Message, len(tt.events))
			errChan := make(chan error)
			mockClient := &MockEventClient{
				EventsChan: eventsChan,
				ErrChan:    errChan,
				Inspect:    tt.containers,
			}
			fetcher := NewDockerEventPortFetcher(mockClient)

			addCh := make(chan types.PortMapping, 10)
			deleteCh := make(chan types.PortMapping, 10)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go fetcher.Listen(ctx, addCh, deleteCh)

			// Send events
			for _, event := range tt.events {
				eventsChan <- event
			}

			// Collect results with timeout
			var gotAdd, gotDelete []types.PortMapping
			timeout := time.After(1 * time.Second)

		collect:
			for {
				select {
				case m := <-addCh:
					gotAdd = append(gotAdd, m)
				case m := <-deleteCh:
					gotDelete = append(gotDelete, m)
				case <-timeout:
					break collect
				}
			}

			assert.ElementsMatch(t, tt.wantAdd, gotAdd)
			assert.ElementsMatch(t, tt.wantDelete, gotDelete)

			if len(gotAdd) > 0 {
				client, err := upnp.NewDummyClient(upnp.DefaultLeaseDuration)
				assert.NoError(t, err)
				err = client.ForwardPorts(gotAdd)
				assert.NoError(t, err)
			}
		})
	}
}
