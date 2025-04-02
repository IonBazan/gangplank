package fetchers

import (
	"context"
	"testing"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

type MockDockerClient struct {
	Containers []container.Summary
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if ctx == nil {
		return nil, assert.AnError
	}
	return m.Containers, nil
}

func TestDockerPortFetcher_FetchPorts(t *testing.T) {
	tests := []struct {
		name       string
		containers []container.Summary
		wantPorts  []types.PortMapping
		wantErr    bool
	}{
		{
			name: "Nginx with published ports",
			containers: []container.Summary{
				{
					ID:    "nginx123",
					Names: []string{"/nginx"},
					Ports: []container.Port{
						{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
					},
					Labels: map[string]string{
						labelForward: "published",
					},
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
			wantErr: false,
		},
		{
			name: "Redis with host-referenced label",
			containers: []container.Summary{
				{
					ID:    "redis456",
					Names: []string{"/redis"},
					Ports: []container.Port{
						{PublicPort: 6379, PrivatePort: 6379, Type: "tcp"},
					},
					Labels: map[string]string{
						labelForward: "6379:6379/tcp",
					},
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
			},
			wantErr: false,
		},
		{
			name: "Postgres with container-referenced label",
			containers: []container.Summary{
				{
					ID:    "pg789",
					Names: []string{"/postgres"},
					Ports: []container.Port{
						{PublicPort: 5433, PrivatePort: 5432, Type: "tcp"}, // Different host port
					},
					Labels: map[string]string{
						labelForwardContainer: "5432/tcp",
					},
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "TCP", Name: "postgres"},
			},
			wantErr: false,
		},
		{
			name:       "No containers",
			containers: []container.Summary{},
			wantPorts:  []types.PortMapping{},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockDockerClient{
				Containers: tt.containers,
			}
			fetcher := NewDockerPortFetcher(mockClient)

			gotPorts, err := fetcher.FetchPorts()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, gotPorts)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.wantPorts, gotPorts)

				client, err := upnp.NewDummyClient(upnp.DefaultLeaseDuration)
				assert.NoError(t, err)
				err = client.ForwardPorts(gotPorts)
				assert.NoError(t, err)
			}
		})
	}
}
