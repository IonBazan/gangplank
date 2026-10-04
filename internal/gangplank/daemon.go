package gangplank

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/IonBazan/gangplank/internal/portmap"
)

const (
	defaultRetryInterval   = 30 * time.Second
	defaultShutdownTimeout = 10 * time.Second
)

type DaemonOptions struct {
	RefreshInterval time.Duration
	Poll            bool
	CleanupOnStop   bool
	CleanupOnExit   bool
	Prune           bool
}

// Daemon keeps the gateway in sync with the providers until its context ends.
type Daemon struct {
	manager *Manager
	connect func(ctx context.Context) (Gateway, error)
	opts    DaemonOptions

	retryInterval   time.Duration
	shutdownTimeout time.Duration
	// last is the set of ports from the previous refresh, so only changes are logged.
	last map[string]portmap.Mapping
}

func NewDaemon(manager *Manager, connect func(ctx context.Context) (Gateway, error), opts DaemonOptions) *Daemon {
	return &Daemon{
		manager:         manager,
		connect:         connect,
		opts:            opts,
		retryInterval:   defaultRetryInterval,
		shutdownTimeout: defaultShutdownTimeout,
	}
}

func (d *Daemon) Run(ctx context.Context) {
	// Connect first, so container events are not dropped for lack of a gateway.
	d.refresh(ctx)

	// A released port may be wanted by another source, so refresh right away.
	released := make(chan struct{}, 1)
	var wg sync.WaitGroup
	if d.opts.Poll {
		wg.Go(func() {
			d.manager.PollAndForward(ctx, d.opts.CleanupOnStop, func() {
				select {
				case released <- struct{}{}:
				default:
				}
			})
		})
	}
	for {
		wait := d.opts.RefreshInterval
		if !d.manager.HasGateway() {
			wait = min(d.retryInterval, d.opts.RefreshInterval)
		}

		select {
		case <-ctx.Done():
			slog.Info("Shutting down")
			wg.Wait()
			d.shutdown()
			return
		case <-time.After(wait):
			slog.Debug("Refreshing port mappings")
			d.refresh(ctx)
		case <-released:
			slog.Debug("Refreshing port mappings after a container stopped")
			d.refresh(ctx)
		}
	}
}

func (d *Daemon) refresh(ctx context.Context) {
	if !d.ensureGateway(ctx) {
		return
	}

	ports, err := d.manager.Refresh(ctx, d.opts.Prune)
	d.logChanges(ports)
	if err != nil {
		slog.Error("Some port mappings could not be refreshed", "error", err)
		d.checkGateway(ctx)
	}
}

// A gateway that stops answering is dropped, so that the next refresh finds it
// again, e.g. after the router restarted with a new address.
func (d *Daemon) checkGateway(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if _, err := d.manager.gateway().GetExternalIP(ctx); err != nil {
		slog.Warn("UPnP gateway is not responding, reconnecting", "error", err)
		d.manager.SetGateway(nil)
	}
}

// Retried on every refresh, so a router that is still booting is picked up later.
func (d *Daemon) ensureGateway(ctx context.Context) bool {
	if d.manager.HasGateway() {
		return true
	}

	gateway, err := d.connect(ctx)
	if err != nil {
		slog.Warn("Failed to connect to UPnP gateway, retrying later", "error", err)
		return false
	}

	slog.Info("Connected to UPnP gateway", "local_ip", gateway.InternalIP())
	d.manager.SetGateway(gateway)
	return true
}

func (d *Daemon) shutdown() {
	if !d.opts.CleanupOnExit {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), d.shutdownTimeout)
	defer cancel()
	if err := d.manager.Cleanup(ctx); err != nil {
		slog.Error("Failed to clean up port mappings", "error", err)
	}
}

func (d *Daemon) logChanges(ports []portmap.Mapping) {
	current := make(map[string]portmap.Mapping, len(ports))
	for _, p := range ports {
		current[p.Key()] = p
		if _, ok := d.last[p.Key()]; !ok {
			LogMapping(p)
		}
	}
	for key, p := range d.last {
		if _, ok := current[key]; !ok {
			slog.Info("Port no longer requested", "port", key, "name", p.Name)
		}
	}
	d.last = current
}

func LogMapping(p portmap.Mapping) {
	slog.Info("Forwarding port", "port", p.Key(), "internal_port", p.InternalPort, "name", p.Name)
}
