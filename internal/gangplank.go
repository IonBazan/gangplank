package internal

import (
	"context"
	"fmt"
	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/providers"
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/docker/docker/client"
	"log"
)

type Gangplank struct {
	PortProviders      []providers.PortProvider
	EventPortProviders []providers.EventPortProvider
	upnpClient         *upnp.Client
}

func NewGangplank(cfg *config.Config, upnpClient *upnp.Client) *Gangplank {
	dockerCli, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}

	return &Gangplank{
		PortProviders: []providers.PortProvider{
			providers.NewConfigPortProvider(cfg),
			providers.NewDockerPortProvider(dockerCli),
		},
		EventPortProviders: []providers.EventPortProvider{
			providers.NewDockerEventPortProvider(dockerCli),
		},
		upnpClient: upnpClient,
	}
}

func (g *Gangplank) GetPortMappings() ([]types.PortMapping, error) {
	allPorts := []types.PortMapping{}
	for _, portProvider := range g.PortProviders {
		ports, err := portProvider.GetPortMappings()
		if err != nil {
			return nil, err
		}
		allPorts = append(allPorts, ports...)
	}
	return allPorts, nil
}

func (g *Gangplank) ForwardPorts(ports []types.PortMapping) error {
	if g.upnpClient == nil {
		log.Println("UPnP client is not initialized, skipping port forwarding.")
		return nil
	}

	return g.upnpClient.ForwardPorts(ports)
}

func (g *Gangplank) PollAndForward(ctx context.Context, cleanup bool) {
	addCh := make(chan types.PortMapping)
	deleteCh := make(chan types.PortMapping)

	for _, provider := range g.EventPortProviders {
		go provider.Listen(ctx, providers.PortEventChannels{Add: addCh, Delete: deleteCh})
	}

	if g.upnpClient != nil && cleanup {
		go func() {
			for m := range deleteCh {
				if err := g.upnpClient.DeletePortMapping(m.ExternalPort, m.Protocol); err != nil {
					log.Printf("Failed to delete port mapping %d/%s for %s: %v", m.ExternalPort, m.Protocol, m.Name, err)
				} else {
					log.Printf("Deleted port mapping %d/%s for %s", m.ExternalPort, m.Protocol, m.Name)
				}
			}
		}()
	}

	for p := range addCh {
		fmt.Printf("New Container Port Mapping (Container: %s): External=%d, Internal=%d, Protocol=%s\n", p.Name, p.ExternalPort, p.InternalPort, p.Protocol)
		if g.upnpClient != nil {
			if err := g.upnpClient.ForwardPorts([]types.PortMapping{p}); err != nil {
				log.Printf("Error forwarding new port: %v", err)
			}
		}
	}
}
