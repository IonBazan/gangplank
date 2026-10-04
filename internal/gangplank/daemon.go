package gangplank

import (
	"context"
	"log"
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

	var wg sync.WaitGroup
	if d.opts.Poll {
		wg.Go(func() { d.manager.PollAndForward(ctx, d.opts.CleanupOnStop) })
	}
	for {
		wait := d.opts.RefreshInterval
		if !d.manager.HasGateway() {
			wait = min(d.retryInterval, d.opts.RefreshInterval)
		}

		select {
		case <-ctx.Done():
			log.Println("Shutting down...")
			wg.Wait()
			d.shutdown()
			return
		case <-time.After(wait):
			log.Printf("Refreshing port mappings...")
			d.refresh(ctx)
		}
	}
}

func (d *Daemon) refresh(ctx context.Context) {
	if !d.ensureGateway(ctx) {
		return
	}

	ports, err := d.manager.GetPortMappings(ctx)
	if err != nil {
		log.Printf("Some port mappings could not be fetched: %v", err)
	}
	LogMappings(ports)
	if err := d.manager.Sync(ctx, ports, d.opts.Prune); err != nil {
		log.Printf("Some port mappings could not be applied: %v", err)
	}
}

// Retried on every refresh, so a router that is still booting is picked up later.
func (d *Daemon) ensureGateway(ctx context.Context) bool {
	if d.manager.HasGateway() {
		return true
	}

	gateway, err := d.connect(ctx)
	if err != nil {
		log.Printf("Failed to initialize UPnP client: %v, retrying later", err)
		return false
	}

	log.Printf("UPnP client initialized with local IP: %s", gateway.InternalIP())
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
		log.Printf("Failed to clean up port mappings: %v", err)
	}
}

func LogMappings(ports []portmap.Mapping) {
	for _, p := range ports {
		if p.Name != "" {
			log.Printf("Port Mapping (Container: %s): External=%d, Internal=%d, Protocol=%s", p.Name, p.ExternalPort, p.InternalPort, p.Protocol)
		} else {
			log.Printf("Port Mapping: External=%d, Internal=%d, Protocol=%s", p.ExternalPort, p.InternalPort, p.Protocol)
		}
	}
}
