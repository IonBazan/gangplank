package providers

import (
	"context"
	"testing"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

type MockDockerClient struct {
	Containers []container.Summary
	Err        error
}

func (m *MockDockerClient) ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Containers, nil
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
					Ports: []container.Port{
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
