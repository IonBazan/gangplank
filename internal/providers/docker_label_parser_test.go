package providers

import (
	"net/netip"
	"testing"

	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/assert"

	"github.com/IonBazan/gangplank/internal/portmap"
)

func TestExtractPortsFromContainer(t *testing.T) {
	tests := []struct {
		name      string
		ctr       container.Summary
		wantPorts []portmap.Mapping
	}{
		{
			name: "Nginx with published ports",
			ctr: container.Summary{
				ID:    "nginx1234567890",
				Names: []string{"/nginx"},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "published",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "nginx"},
			},
		},
		{
			name: "Redis with host-referenced label",
			ctr: container.Summary{
				ID:    "redis4567890123",
				Names: []string{"/redis"},
				Ports: []container.PortSummary{
					{PublicPort: 6379, PrivatePort: 6379, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "6379:6379/tcp",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 6379, InternalPort: 6379, Protocol: "TCP", Name: "redis"},
			},
		},
		{
			name: "Postgres with container-referenced label",
			ctr: container.Summary{
				ID:    "pg789012345678",
				Names: []string{"/postgres"},
				Ports: []container.PortSummary{
					{PublicPort: 5433, PrivatePort: 5432, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForwardContainer: "5432/tcp",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 5432, InternalPort: 5433, Protocol: "TCP", Name: "postgres"},
			},
		},
		{
			name: "Multiple host-referenced mappings",
			ctr: container.Summary{
				ID:    "nginx_multi12345",
				Names: []string{"/nginx-multi"},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
					{PublicPort: 8443, PrivatePort: 443, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "8080:80/tcp, 8443:443/tcp",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "nginx-multi"},
				{ExternalPort: 8443, InternalPort: 443, Protocol: "TCP", Name: "nginx-multi"},
			},
		},
		{
			name: "No labels",
			ctr: container.Summary{
				ID:    "no_labels123456",
				Names: []string{"/no-labels"},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{},
			},
			wantPorts: []portmap.Mapping{},
		},
		{
			name: "Invalid host-referenced label",
			ctr: container.Summary{
				ID:    "invalid123456789",
				Names: []string{"/invalid"},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "invalid:port/tcp",
				},
			},
			wantPorts: []portmap.Mapping{},
		},
		{
			name: "Container-referenced no matching port",
			ctr: container.Summary{
				ID:    "no_match12345678",
				Names: []string{"/no-match"},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForwardContainer: "9999/tcp",
				},
			},
			wantPorts: []portmap.Mapping{},
		},
		{
			name: "Container-referenced label with explicit external port",
			ctr: container.Summary{
				ID:    "web123456789012",
				Names: []string{"/web"},
				Ports: []container.PortSummary{
					{PublicPort: 32768, PrivatePort: 80, Type: "tcp"},
					{PublicPort: 32769, PrivatePort: 53, Type: "udp"},
					{PublicPort: 32770, PrivatePort: 53, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForwardContainer: "8080:80/tcp, 5353:53/udp",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 32768, Protocol: "TCP", Name: "web"},
				{ExternalPort: 5353, InternalPort: 32769, Protocol: "UDP", Name: "web"},
			},
		},
		{
			name: "Duplicate IPv4 and IPv6 bindings, loopback skipped",
			ctr: container.Summary{
				ID:    "dual12345678901",
				Names: []string{"/dual"},
				Ports: []container.PortSummary{
					{IP: netip.MustParseAddr("0.0.0.0"), PublicPort: 80, PrivatePort: 80, Type: "tcp"},
					{IP: netip.MustParseAddr("::"), PublicPort: 80, PrivatePort: 80, Type: "tcp"},
					{IP: netip.MustParseAddr("127.0.0.1"), PublicPort: 9000, PrivatePort: 9000, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "published",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 80, InternalPort: 80, Protocol: "TCP", Name: "dual"},
			},
		},
		{
			name: "Container-referenced label edge cases",
			ctr: container.Summary{
				ID:    "edge1234567890",
				Names: []string{"/edge"},
				Ports: []container.PortSummary{
					{PublicPort: 32768, PrivatePort: 80, Type: "tcp"},
					{IP: netip.MustParseAddr("127.0.0.1"), PublicPort: 32769, PrivatePort: 81, Type: "tcp"},
				},
				Labels: map[string]string{
					// Invalid spec, wrong protocol, loopback-only binding and empty entries are skipped.
					labelForwardContainer: "abc/tcp, 80/udp, 81/tcp, , 80",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 80, InternalPort: 32768, Protocol: "TCP", Name: "edge"},
			},
		},
		{
			name: "Published keyword in container-referenced label",
			ctr: container.Summary{
				ID:    "pub12345678901",
				Names: []string{"/pub"},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
					{PrivatePort: 9000, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForwardContainer: "published",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "pub"},
			},
		},
		{
			name: "Both labels requesting the same port are merged",
			ctr: container.Summary{
				ID:    "both1234567890",
				Names: []string{"/both"},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward:          "8080",
					labelForwardContainer: "8080:80",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "both"},
			},
		},
		{
			name: "Short ID without name",
			ctr: container.Summary{
				ID:    "short123",
				Names: []string{},
				Ports: []container.PortSummary{
					{PublicPort: 8080, PrivatePort: 80, Type: "tcp"},
				},
				Labels: map[string]string{
					labelForward: "published",
				},
			},
			wantPorts: []portmap.Mapping{
				{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", Name: "short123"},
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
