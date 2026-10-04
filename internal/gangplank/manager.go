package gangplank

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
	GetExternalIP(ctx context.Context) (string, error)
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

	// syncMu serialises refreshes and container events, so that a refresh which
	// fetched the ports before a container stopped cannot open its ports again.
	syncMu sync.Mutex

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

// Refresh fetches the wanted mappings from all sources and forwards them.
// With prune, Gangplank mappings for this host that are no longer wanted are deleted.
func (mgr *Manager) Refresh(ctx context.Context, prune bool) ([]portmap.Mapping, error) {
	mgr.syncMu.Lock()
	defer mgr.syncMu.Unlock()

	gw := mgr.gateway()
	if gw == nil {
		slog.Debug("No UPnP gateway yet, skipping port forwarding")
		return nil, nil
	}

	ports, fetchErr := mgr.fetch(ctx)
	complete := fetchErr == nil
	mgr.track(ports, complete)

	err := gw.ForwardPorts(ctx, ports)
	// A failing source returns only part of the ports, so pruning would close the rest.
	if prune && complete {
		err = errors.Join(err, mgr.prune(ctx, gw, ports))
	}

	return ports, errors.Join(fetchErr, err)
}

// A failing provider does not drop the others' mappings. When several sources
// claim the same external port and protocol, the first one wins.
func (mgr *Manager) fetch(ctx context.Context) ([]portmap.Mapping, error) {
	slog.Debug("Fetching port mappings")
	allPorts := []portmap.Mapping{}
	wanted := map[string]portmap.Mapping{}
	var errs []error

	for _, portProvider := range mgr.portProviders {
		ports, err := portProvider.GetPortMappings(ctx)
		if err != nil {
			errs = append(errs, err)
		}
		for _, p := range ports {
			p = p.Normalize()
			if owner, taken := heldByOther(wanted, p); taken {
				slog.Warn("Port requested twice, keeping the first", "port", p.Key(), "kept", owner.Name, "ignored", p.Name)
				continue
			}
			if _, seen := wanted[p.Key()]; !seen {
				wanted[p.Key()] = p
				allPorts = append(allPorts, p)
			}
		}
	}

	slog.Debug("Fetched port mappings", "count", len(allPorts))
	return allPorts, errors.Join(errs...)
}

// A partial set, from a failing source, is merged so that the ports this source
// opened earlier are still known for ownership and cleanup.
func (mgr *Manager) track(ports []portmap.Mapping, complete bool) {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if complete || mgr.forwarded == nil {
		mgr.forwarded = make(map[string]portmap.Mapping, len(ports))
	}
	for _, p := range ports {
		mgr.forwarded[p.Key()] = p
	}
}

func (mgr *Manager) prune(ctx context.Context, gw Gateway, ports []portmap.Mapping) error {
	entries, err := gw.ListPortMappings(ctx)
	if err != nil {
		return fmt.Errorf("failed to list gateway mappings: %w", err)
	}

	keep := make(map[string]bool, len(ports))
	for _, p := range ports {
		keep[p.Key()] = true
	}

	var errs []error
	for _, e := range entries {
		if !e.IsOwned() || e.InternalIP != gw.InternalIP() {
			continue
		}
		key := portmap.Mapping{ExternalPort: e.ExternalPort, Protocol: strings.ToUpper(e.Protocol)}.Key()
		if keep[key] {
			continue
		}
		if err := gw.DeletePortMapping(ctx, e.ExternalPort, e.Protocol); err != nil {
			errs = append(errs, fmt.Errorf("prune %s: %w", key, err))
			continue
		}
		slog.Info("Pruned stale port mapping", "port", key, "description", e.Description)
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
		slog.Info("Deleted port mapping", "port", m.Key(), "name", m.Name)
	}

	return errors.Join(errs...)
}

// PollAndForward forwards the ports of containers as they start. When a
// container stops, its ports are released, closed with cleanup, and released
// is called so that another source waiting for them can get them.
func (mgr *Manager) PollAndForward(ctx context.Context, cleanup bool, released func()) {
	addCh := make(chan portmap.Mapping)
	removeCh := make(chan portmap.Mapping)

	var wg sync.WaitGroup
	for _, provider := range mgr.eventProviders {
		wg.Go(func() {
			provider.Listen(ctx, providers.PortEventChannels{Add: addCh, Delete: removeCh})
		})
	}
	defer wg.Wait()

	for {
		select {
		case <-ctx.Done():
			return
		case m := <-addCh:
			mgr.add(ctx, m)
		case m := <-removeCh:
			if mgr.remove(ctx, m, cleanup) {
				released()
			}
		}
	}
}

// heldByOther returns the mapping in set that holds m's port for another source.
func heldByOther(set map[string]portmap.Mapping, m portmap.Mapping) (portmap.Mapping, bool) {
	owner, ok := set[m.Key()]
	return owner, ok && owner.Name != m.Name
}

// claim records m as forwarded unless another source already owns its port.
func (mgr *Manager) claim(m portmap.Mapping) bool {
	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	if owner, taken := heldByOther(mgr.forwarded, m); taken {
		slog.Warn("Port already in use, ignoring", "port", m.Key(), "owner", owner.Name, "ignored", m.Name)
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

	if owner, taken := heldByOther(mgr.forwarded, m); taken {
		slog.Info("Port still in use, keeping it", "port", m.Key(), "owner", owner.Name)
		return false
	}
	delete(mgr.forwarded, m.Key())
	return true
}

func (mgr *Manager) add(ctx context.Context, m portmap.Mapping) {
	mgr.syncMu.Lock()
	defer mgr.syncMu.Unlock()

	gw := mgr.gateway()
	if gw == nil {
		slog.Debug("No UPnP gateway yet, skipping port forwarding")
		return
	}

	m = m.Normalize()
	if !mgr.claim(m) {
		return
	}
	slog.Info("Forwarding port for started container", "port", m.Key(), "internal_port", m.InternalPort, "container", m.Name)
	if err := gw.ForwardPorts(ctx, []portmap.Mapping{m}); err != nil {
		slog.Error("Failed to forward port", "port", m.Key(), "error", err)
	}
}

// remove releases the port of a stopped container and, with cleanup, closes it.
// It reports whether the port was released.
func (mgr *Manager) remove(ctx context.Context, m portmap.Mapping, cleanup bool) bool {
	mgr.syncMu.Lock()
	defer mgr.syncMu.Unlock()

	m = m.Normalize()
	if !mgr.release(m) {
		return false
	}

	gw := mgr.gateway()
	if !cleanup || gw == nil {
		return true
	}
	if err := gw.DeletePortMapping(ctx, m.ExternalPort, m.Protocol); err != nil {
		slog.Error("Failed to delete port mapping", "port", m.Key(), "container", m.Name, "error", err)
	} else {
		slog.Info("Deleted port mapping", "port", m.Key(), "name", m.Name)
	}
	return true
}
