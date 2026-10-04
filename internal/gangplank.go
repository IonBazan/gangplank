package internal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/providers"
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
)

type Forwarder interface {
	ForwardPorts(ctx context.Context, mappings []types.PortMapping) error
	DeletePortMapping(ctx context.Context, externalPort int, protocol string) error
	ListPortMappings(ctx context.Context) ([]upnp.PortMappingEntry, error)
	InternalIP() string
}

type DockerClient interface {
	providers.EventInspector
}

type Gangplank struct {
	PortProviders      []providers.PortProvider
	EventPortProviders []providers.EventPortProvider

	mu        sync.Mutex
	upnp      Forwarder
	forwarded map[string]types.PortMapping
}

func NewGangplank(cfg *config.Config, dockerCli DockerClient) *Gangplank {
	return &Gangplank{
		PortProviders: []providers.PortProvider{
			providers.NewConfigPortProvider(cfg),
			providers.NewDockerPortProvider(dockerCli),
		},
		EventPortProviders: []providers.EventPortProvider{
			providers.NewDockerEventPortProvider(dockerCli),
		},
	}
}

func (g *Gangplank) SetForwarder(f Forwarder) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.upnp = f
}

func (g *Gangplank) HasForwarder() bool {
	return g.forwarder() != nil
}

func (g *Gangplank) forwarder() Forwarder {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.upnp
}

// A failing provider does not drop the others' mappings. When several sources
// claim the same external port and protocol, the first one wins.
func (g *Gangplank) GetPortMappings(ctx context.Context) ([]types.PortMapping, error) {
	log.Println("Fetching port mappings...")
	allPorts := []types.PortMapping{}
	owners := map[string]string{}
	var errs []error

	for _, portProvider := range g.PortProviders {
		ports, err := portProvider.GetPortMappings(ctx)
		if err != nil {
			log.Printf("Error fetching port mappings: %v", err)
			errs = append(errs, err)
		}
		for _, p := range ports {
			p = p.Normalize()
			if owner, taken := owners[p.Key()]; taken {
				if owner != p.Name {
					log.Printf("Port %s is requested by both %q and %q, keeping %q", p.Key(), owner, p.Name, owner)
				}
				continue
			}
			owners[p.Key()] = p.Name
			allPorts = append(allPorts, p)
		}
	}

	log.Printf("Fetched %d port mappings", len(allPorts))
	return allPorts, errors.Join(errs...)
}

func (g *Gangplank) ForwardPorts(ctx context.Context, ports []types.PortMapping) error {
	f := g.forwarder()
	if f == nil {
		log.Println("UPnP client is not initialized, skipping port forwarding.")
		return nil
	}

	err := f.ForwardPorts(ctx, ports)
	g.mu.Lock()
	if g.forwarded == nil {
		g.forwarded = map[string]types.PortMapping{}
	}
	for _, p := range ports {
		p = p.Normalize()
		g.forwarded[p.Key()] = p
	}
	g.mu.Unlock()

	return err
}

// With prune, Gangplank mappings for this host that are no longer desired are deleted.
func (g *Gangplank) Sync(ctx context.Context, desired []types.PortMapping, prune bool) error {
	if g.forwarder() == nil {
		log.Println("UPnP client is not initialized, skipping port forwarding.")
		return nil
	}

	err := g.ForwardPorts(ctx, desired)

	g.mu.Lock()
	keep := make(map[string]types.PortMapping, len(desired))
	for _, p := range desired {
		p = p.Normalize()
		keep[p.Key()] = p
	}
	g.forwarded = keep
	g.mu.Unlock()

	if prune {
		err = errors.Join(err, g.prune(ctx, keep))
	}

	return err
}

func (g *Gangplank) prune(ctx context.Context, keep map[string]types.PortMapping) error {
	f := g.forwarder()
	entries, err := f.ListPortMappings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list gateway mappings: %w", err)
	}

	var errs []error
	for _, e := range entries {
		if !e.IsOwned() || e.InternalIP != f.InternalIP() {
			continue
		}
		key := types.PortMapping{ExternalPort: e.ExternalPort, Protocol: strings.ToUpper(e.Protocol)}.Key()
		if _, ok := keep[key]; ok {
			continue
		}
		if err := f.DeletePortMapping(ctx, e.ExternalPort, e.Protocol); err != nil {
			errs = append(errs, fmt.Errorf("prune %s: %w", key, err))
			continue
		}
		log.Printf("Pruned stale port mapping %s (%s)", key, e.Description)
	}

	return errors.Join(errs...)
}

func (g *Gangplank) Cleanup(ctx context.Context) error {
	f := g.forwarder()
	if f == nil {
		return nil
	}

	g.mu.Lock()
	mappings := make([]types.PortMapping, 0, len(g.forwarded))
	for _, m := range g.forwarded {
		mappings = append(mappings, m)
	}
	g.forwarded = nil
	g.mu.Unlock()

	var errs []error
	for _, m := range mappings {
		if err := f.DeletePortMapping(ctx, m.ExternalPort, m.Protocol); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", m.Key(), err))
			continue
		}
		log.Printf("Deleted port mapping %s for %s", m.Key(), m.Name)
	}

	return errors.Join(errs...)
}

func (g *Gangplank) PollAndForward(ctx context.Context, cleanup bool) {
	addCh := make(chan types.PortMapping)
	var deleteCh chan types.PortMapping
	if cleanup {
		deleteCh = make(chan types.PortMapping)
	}

	var wg sync.WaitGroup
	for _, provider := range g.EventPortProviders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			provider.Listen(ctx, providers.PortEventChannels{Add: addCh, Delete: deleteCh})
		}()
	}
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case p := <-addCh:
			log.Printf("New container port mapping (Container: %s): External=%d, Internal=%d, Protocol=%s", p.Name, p.ExternalPort, p.InternalPort, p.Protocol)
			if err := g.ForwardPorts(ctx, []types.PortMapping{p}); err != nil {
				log.Printf("Error forwarding new port: %v", err)
			}
		case m := <-deleteCh:
			g.delete(ctx, m)
		}
	}
}

func (g *Gangplank) delete(ctx context.Context, m types.PortMapping) {
	f := g.forwarder()
	if f == nil {
		return
	}

	m = m.Normalize()
	g.mu.Lock()
	delete(g.forwarded, m.Key())
	g.mu.Unlock()

	if err := f.DeletePortMapping(ctx, m.ExternalPort, m.Protocol); err != nil {
		log.Printf("Failed to delete port mapping %s for %s: %v", m.Key(), m.Name, err)
	} else {
		log.Printf("Deleted port mapping %s for %s", m.Key(), m.Name)
	}
}
