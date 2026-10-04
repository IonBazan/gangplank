package gangplank

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/portmap"
	"github.com/IonBazan/gangplank/internal/providers"
	"github.com/IonBazan/gangplank/internal/upnp"
)

type Gateway interface {
	ForwardPorts(ctx context.Context, mappings []portmap.Mapping) error
	DeletePortMapping(ctx context.Context, externalPort int, protocol string) error
	ListPortMappings(ctx context.Context) ([]upnp.PortMappingEntry, error)
	InternalIP() string
}

type DockerClient interface {
	providers.EventInspector
}

type Manager struct {
	PortProviders      []providers.PortProvider
	EventPortProviders []providers.EventPortProvider

	mu        sync.Mutex
	gw        Gateway
	forwarded map[string]portmap.Mapping
}

func NewManager(cfg *config.Config, dockerCli DockerClient) *Manager {
	return &Manager{
		PortProviders: []providers.PortProvider{
			providers.NewConfigPortProvider(cfg),
			providers.NewDockerPortProvider(dockerCli),
		},
		EventPortProviders: []providers.EventPortProvider{
			providers.NewDockerEventPortProvider(dockerCli),
		},
	}
}

func (mgr *Manager) SetGateway(gw Gateway) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	mgr.gw = gw
}

func (mgr *Manager) HasGateway() bool {
	return mgr.gateway() != nil
}

func (mgr *Manager) gateway() Gateway {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()
	return mgr.gw
}

// A failing provider does not drop the others' mappings. When several sources
// claim the same external port and protocol, the first one wins.
func (mgr *Manager) GetPortMappings(ctx context.Context) ([]portmap.Mapping, error) {
	log.Println("Fetching port mappings...")
	allPorts := []portmap.Mapping{}
	owners := map[string]string{}
	var errs []error

	for _, portProvider := range mgr.PortProviders {
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

func (mgr *Manager) ForwardPorts(ctx context.Context, ports []portmap.Mapping) error {
	gw := mgr.gateway()
	if gw == nil {
		log.Println("UPnP client is not initialized, skipping port forwarding.")
		return nil
	}

	err := gw.ForwardPorts(ctx, ports)
	mgr.mu.Lock()
	if mgr.forwarded == nil {
		mgr.forwarded = map[string]portmap.Mapping{}
	}
	for _, p := range ports {
		p = p.Normalize()
		mgr.forwarded[p.Key()] = p
	}
	mgr.mu.Unlock()

	return err
}

// With prune, Gangplank mappings for this host that are no longer desired are deleted.
func (mgr *Manager) Sync(ctx context.Context, desired []portmap.Mapping, prune bool) error {
	if mgr.gateway() == nil {
		log.Println("UPnP client is not initialized, skipping port forwarding.")
		return nil
	}

	err := mgr.ForwardPorts(ctx, desired)

	mgr.mu.Lock()
	keep := make(map[string]portmap.Mapping, len(desired))
	for _, p := range desired {
		p = p.Normalize()
		keep[p.Key()] = p
	}
	mgr.forwarded = keep
	mgr.mu.Unlock()

	if prune {
		err = errors.Join(err, mgr.prune(ctx, keep))
	}

	return err
}

func (mgr *Manager) prune(ctx context.Context, keep map[string]portmap.Mapping) error {
	gw := mgr.gateway()
	entries, err := gw.ListPortMappings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list gateway mappings: %w", err)
	}

	var errs []error
	for _, e := range entries {
		if !e.IsOwned() || e.InternalIP != gw.InternalIP() {
			continue
		}
		key := portmap.Mapping{ExternalPort: e.ExternalPort, Protocol: strings.ToUpper(e.Protocol)}.Key()
		if _, ok := keep[key]; ok {
			continue
		}
		if err := gw.DeletePortMapping(ctx, e.ExternalPort, e.Protocol); err != nil {
			errs = append(errs, fmt.Errorf("prune %s: %w", key, err))
			continue
		}
		log.Printf("Pruned stale port mapping %s (%s)", key, e.Description)
	}

	return errors.Join(errs...)
}

func (mgr *Manager) Cleanup(ctx context.Context) error {
	gw := mgr.gateway()
	if gw == nil {
		return nil
	}

	mgr.mu.Lock()
	mappings := make([]portmap.Mapping, 0, len(mgr.forwarded))
	for _, m := range mgr.forwarded {
		mappings = append(mappings, m)
	}
	mgr.forwarded = nil
	mgr.mu.Unlock()

	var errs []error
	for _, m := range mappings {
		if err := gw.DeletePortMapping(ctx, m.ExternalPort, m.Protocol); err != nil {
			errs = append(errs, fmt.Errorf("delete %s: %w", m.Key(), err))
			continue
		}
		log.Printf("Deleted port mapping %s for %s", m.Key(), m.Name)
	}

	return errors.Join(errs...)
}

func (mgr *Manager) PollAndForward(ctx context.Context, cleanup bool) {
	addCh := make(chan portmap.Mapping)
	var deleteCh chan portmap.Mapping
	if cleanup {
		deleteCh = make(chan portmap.Mapping)
	}

	var wg sync.WaitGroup
	for _, provider := range mgr.EventPortProviders {
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
			if err := mgr.ForwardPorts(ctx, []portmap.Mapping{p}); err != nil {
				log.Printf("Error forwarding new port: %v", err)
			}
		case m := <-deleteCh:
			mgr.delete(ctx, m)
		}
	}
}

func (mgr *Manager) delete(ctx context.Context, m portmap.Mapping) {
	gw := mgr.gateway()
	if gw == nil {
		return
	}

	m = m.Normalize()
	mgr.mu.Lock()
	delete(mgr.forwarded, m.Key())
	mgr.mu.Unlock()

	if err := gw.DeletePortMapping(ctx, m.ExternalPort, m.Protocol); err != nil {
		log.Printf("Failed to delete port mapping %s for %s: %v", m.Key(), m.Name, err)
	} else {
		log.Printf("Deleted port mapping %s for %s", m.Key(), m.Name)
	}
}
