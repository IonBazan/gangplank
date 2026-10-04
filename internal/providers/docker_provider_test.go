package providers

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"

	"github.com/IonBazan/gangplank/internal/types"
)

type MockDockerClient struct {
	Containers []container.Summary
	Err        error
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error) {
	if m.Err != nil {
		return client.ContainerListResult{}, m.Err
	}
	return client.ContainerListResult{Items: m.Containers}, nil
}

func TestDockerPortProvider_GetPortMappings(t *testing.T) {
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
					Ports: []container.PortSummary{
						{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
					},
					Labels: map[string]string{
						labelForward: "published",
					},
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "nginx"},
			},
			wantErr: false,
		},
		{
			name: "Redis with host-referenced label",
			containers: []container.Summary{
				{
					ID:    "redis456",
					Names: []string{"/redis"},
					Ports: []container.PortSummary{
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
					Ports: []container.PortSummary{
						{PublicPort: 5433, PrivatePort: 5432, Type: "tcp"},
					},
					Labels: map[string]string{
						labelForwardContainer: "5432/tcp",
					},
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 5432, InternalPort: 5433, Protocol: "TCP", Name: "postgres"},
			},
			wantErr: false,
		},
		{
			name:    "Docker unavailable",
			wantErr: true,
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
			if tt.wantErr {
				mockClient.Err = assert.AnError
			}
			portProvider := NewDockerPortProvider(mockClient)

			gotPorts, err := portProvider.GetPortMappings(context.Background())
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, gotPorts)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.wantPorts, gotPorts)
			}
		})
	}
}

func TestDockerPortProvider_SortsByContainerName(t *testing.T) {
	mockClient := &MockDockerClient{Containers: []container.Summary{
		{ID: "z", Names: []string{"/zeta"}, Labels: map[string]string{labelForward: "443, 80"}},
		{ID: "a", Names: []string{"/alpha"}, Labels: map[string]string{labelForward: "8080"}},
	}}

	got, err := NewDockerPortProvider(mockClient).GetPortMappings(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, []types.PortMapping{
		{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "alpha"},
		{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", Name: "zeta"},
		{ExternalPort: 443, InternalPort: 443, Protocol: "TCP", Name: "zeta"},
	}, got)
}
