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

// Manager keeps track of which source owns each forwarded port, so that one
// source never removes or replaces a port that another source asked for.
type Manager struct {
	portProviders  []providers.PortProvider
	eventProviders []providers.EventPortProvider

	mu        sync.Mutex
	gw        Gateway
	forwarded map[string]portmap.Mapping
}

func NewManager(cfg *config.Config, dockerCli DockerClient) *Manager {
	return &Manager{
		portProviders: []providers.PortProvider{
			providers.NewConfigPortProvider(cfg),
			providers.NewDockerPortProvider(dockerCli),
		},
		eventProviders: []providers.EventPortProvider{
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

	for _, portProvider := range mgr.portProviders {
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

// Sync forwards the desired mappings and makes them the current set.
// With prune, Gangplank mappings for this host that are no longer desired are deleted.
func (mgr *Manager) Sync(ctx context.Context, desired []portmap.Mapping, prune bool) error {
	gw := mgr.gateway()
	if gw == nil {
		log.Println("UPnP client is not initialized, skipping port forwarding.")
		return nil
	}

	keep := make(map[string]portmap.Mapping, len(desired))
	for _, p := range desired {
		p = p.Normalize()
		keep[p.Key()] = p
	}
	mgr.mu.Lock()
	mgr.forwarded = keep
	mgr.mu.Unlock()

	err := gw.ForwardPorts(ctx, desired)
	if prune {
		err = errors.Join(err, mgr.prune(ctx, gw, keep))
	}

	return err
}

func (mgr *Manager) prune(ctx context.Context, gw Gateway, keep map[string]portmap.Mapping) error {
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
	for _, provider := range mgr.eventProviders {
		wg.Go(func() {
			provider.Listen(ctx, providers.PortEventChannels{Add: addCh, Delete: deleteCh})
		})
	}
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case m := <-addCh:
			mgr.add(ctx, m)
		case m := <-deleteCh:
			mgr.delete(ctx, m)
		}
	}
}

// claim records m as forwarded unless another source already owns its port.
func (mgr *Manager) claim(m portmap.Mapping) bool {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if owner, ok := mgr.forwarded[m.Key()]; ok && owner.Name != m.Name {
		log.Printf("Port %s is already used by %q, ignoring %q", m.Key(), owner.Name, m.Name)
		return false
	}
	if mgr.forwarded == nil {
		mgr.forwarded = map[string]portmap.Mapping{}
	}
	mgr.forwarded[m.Key()] = m
	return true
}

// release forgets m unless its port belongs to another source.
func (mgr *Manager) release(m portmap.Mapping) bool {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if owner, ok := mgr.forwarded[m.Key()]; ok && owner.Name != m.Name {
		log.Printf("Port %s is still used by %q, keeping it", m.Key(), owner.Name)
		return false
	}
	delete(mgr.forwarded, m.Key())
	return true
}

func (mgr *Manager) add(ctx context.Context, m portmap.Mapping) {
	gw := mgr.gateway()
	if gw == nil {
		log.Println("UPnP client is not initialized, skipping port forwarding.")
		return
	}

	m = m.Normalize()
	log.Printf("New container port mapping (Container: %s): External=%d, Internal=%d, Protocol=%s", m.Name, m.ExternalPort, m.InternalPort, m.Protocol)
	if !mgr.claim(m) {
		return
	}
	if err := gw.ForwardPorts(ctx, []portmap.Mapping{m}); err != nil {
		log.Printf("Error forwarding new port: %v", err)
	}
}

func (mgr *Manager) delete(ctx context.Context, m portmap.Mapping) {
	gw := mgr.gateway()
	if gw == nil {
		return
	}

	m = m.Normalize()
	if !mgr.release(m) {
		return
	}
	if err := gw.DeletePortMapping(ctx, m.ExternalPort, m.Protocol); err != nil {
		log.Printf("Failed to delete port mapping %s for %s: %v", m.Key(), m.Name, err)
	} else {
		log.Printf("Deleted port mapping %s for %s", m.Key(), m.Name)
	}
}
