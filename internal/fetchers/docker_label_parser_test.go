package fetchers

import (
	"testing"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/docker/docker/api/types/container"
	"github.com/stretchr/testify/assert"
)

func TestExtractPortsFromContainer(t *testing.T) {
	tests := []struct {
		name      string
		ctr       container.Summary
		wantPorts []types.PortMapping
	}{
		{
			name: "Nginx with published ports",
			ctr: container.Summary{
				ID:    "nginx1234567890",
				Names: []string{"/nginx"},
				Ports: []container.Port{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "published",
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx"},
			},
		},
		{
			name: "Redis with host-referenced label",
			ctr: container.Summary{
				ID:    "redis4567890123",
				Names: []string{"/redis"},
				Ports: []container.Port{
					{PublicPort: 6379, PrivatePort: 6379, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "6379:6379/tcp",
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
			},
		},
		{
			name: "Postgres with container-referenced label",
			ctr: container.Summary{
				ID:    "pg789012345678",
				Names: []string{"/postgres"},
				Ports: []container.Port{
					{PublicPort: 5433, PrivatePort: 5432, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForwardContainer: "5432/tcp",
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 5433, InternalPort: 5432, Protocol: "TCP", Name: "postgres"},
			},
		},
		{
			name: "Multiple host-referenced mappings",
			ctr: container.Summary{
				ID:    "nginx_multi12345",
				Names: []string{"/nginx-multi"},
				Ports: []container.Port{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
					{PublicPort: 8443, PrivatePort: 443, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "8080:80/tcp, 8443:443/tcp",
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx-multi"},
				{ExternalPort: 8443, InternalPort: 443, Protocol: "TCP", Name: "nginx-multi"},
			},
		},
		{
			name: "No labels",
			ctr: container.Summary{
				ID:    "no_labels123456",
				Names: []string{"/no-labels"},
				Ports: []container.Port{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{},
			},
			wantPorts: []types.PortMapping{},
		},
		{
			name: "Invalid host-referenced label",
			ctr: container.Summary{
				ID:    "invalid123456789",
				Names: []string{"/invalid"},
				Ports: []container.Port{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "invalid:port/tcp",
				},
			},
			wantPorts: []types.PortMapping{},
		},
		{
			name: "Container-referenced no matching port",
			ctr: container.Summary{
				ID:    "no_match12345678",
				Names: []string{"/no-match"},
				Ports: []container.Port{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForwardContainer: "9999/tcp",
				},
			},
			wantPorts: []types.PortMapping{},
		},
		{
			name: "Short ID without name",
			ctr: container.Summary{
				ID:    "short123",
				Names: []string{},
				Ports: []container.Port{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "published",
				},
			},
			wantPorts: []types.PortMapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "short123"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPorts := extractPortsFromContainer(tt.ctr)
			assert.ElementsMatch(t, tt.wantPorts, gotPorts)
		})
	}
}
