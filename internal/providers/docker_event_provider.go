package providers

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/docker/docker/api/types/container"
	dockerevents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

const (
	defaultRetryDelay = time.Second
	maxRetryDelay     = 30 * time.Second
)

// EventInspector defines the minimal interface for DockerEventPortProvider.
type EventInspector interface {
	ContainerLister
	Events(ctx context.Context, options dockerevents.ListOptions) (<-chan dockerevents.Message, <-chan error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

// DockerEventPortProvider emits mappings as containers start and stop.
// It remembers what each container exposed when it started, because Docker
// clears port bindings once a container stops.
type DockerEventPortProvider struct {
	dockerCli  EventInspector
	retryDelay time.Duration

	mu      sync.Mutex
	tracked map[string][]types.PortMapping
}

func NewDockerEventPortProvider(cli EventInspector) *DockerEventPortProvider {
	return &DockerEventPortProvider{
		dockerCli:  cli,
		retryDelay: defaultRetryDelay,
		tracked:    map[string][]types.PortMapping{},
	}
}

// Listen streams Docker events until ctx is cancelled, reconnecting with
// backoff when the stream fails. After a reconnect it resyncs running containers
// so events missed while disconnected are not lost.
func (d *DockerEventPortProvider) Listen(ctx context.Context, events PortEventChannels) {
	delay := d.retryDelay
	first := true
	for {
		connected, err := d.listenOnce(ctx, events, !first)
		if ctx.Err() != nil {
			return
		}
		if connected {
			first = false
			delay = d.retryDelay
		}
		log.Printf("Docker event stream interrupted: %v, reconnecting in %s", err, delay)

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maxRetryDelay)
	}
}

func (d *DockerEventPortProvider) listenOnce(ctx context.Context, events PortEventChannels, resync bool) (bool, error) {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Subscribe before syncing so that no event falls between the two.
	eventChan, errChan := d.dockerCli.Events(streamCtx, dockerevents.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("type", "container"),
			filters.Arg("event", "start"),
			filters.Arg("event", "stop"),
			filters.Arg("event", "die"),
		),
	})

	if err := d.sync(ctx, events, resync); err != nil {
		return false, err
	}

	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				return true, errors.New("event stream closed")
			}
			switch event.Action {
			case dockerevents.ActionStart:
				d.handleContainerStart(ctx, event.Actor.ID, events.Add)
			case dockerevents.ActionStop, dockerevents.ActionDie:
				d.handleContainerStop(ctx, event.Actor.ID, events.Delete)
			}
		case err := <-errChan:
			if err == nil {
				err = errors.New("event stream closed")
			}
			return true, err
		case <-ctx.Done():
			return true, nil
		}
	}
}

// sync records the mappings of running containers. When emit is set, it also
// reports containers that started or stopped since the previous sync.
func (d *DockerEventPortProvider) sync(ctx context.Context, events PortEventChannels, emit bool) error {
	current, err := listContainerMappings(ctx, d.dockerCli)
	if err != nil {
		return err
	}

	running := make(map[string]bool, len(current))
	for _, ctr := range current {
		running[ctr.id] = true
		d.mu.Lock()
		_, known := d.tracked[ctr.id]
		d.tracked[ctr.id] = ctr.mappings
		d.mu.Unlock()
		if emit && !known {
			send(ctx, events.Add, ctr.mappings)
		}
	}

	d.mu.Lock()
	var stopped []types.PortMapping
	for id, mappings := range d.tracked {
		if !running[id] {
			stopped = append(stopped, mappings...)
			delete(d.tracked, id)
		}
	}
	d.mu.Unlock()
	if emit {
		send(ctx, events.Delete, stopped)
	}

	return nil
}

func (d *DockerEventPortProvider) handleContainerStart(ctx context.Context, containerID string, addCh chan<- types.PortMapping) {
	info, err := d.dockerCli.ContainerInspect(ctx, containerID)
	if err != nil {
		log.Printf("Failed to inspect container %s: %v", shortID(containerID), err)
		return
	}

	var labels map[string]string
	if info.Config != nil {
		labels = info.Config.Labels
	}
	var name string
	if info.ContainerJSONBase != nil {
		name = info.Name
	}

	mappings := extractPortsFromContainer(container.Summary{
		ID:     containerID,
		Names:  []string{name},
		Labels: labels,
		Ports:  portsFromInspect(info),
	})

	d.mu.Lock()
	d.tracked[containerID] = mappings
	d.mu.Unlock()

	send(ctx, addCh, mappings)
}

func (d *DockerEventPortProvider) handleContainerStop(ctx context.Context, containerID string, deleteCh chan<- types.PortMapping) {
	// Both "stop" and "die" fire for one container; only the first finds it tracked.
	d.mu.Lock()
	mappings, ok := d.tracked[containerID]
	delete(d.tracked, containerID)
	d.mu.Unlock()

	if ok {
		send(ctx, deleteCh, mappings)
	}
}

func send(ctx context.Context, ch chan<- types.PortMapping, mappings []types.PortMapping) {
	if ch == nil {
		return
	}
	for _, m := range mappings {
		select {
		case ch <- m:
		case <-ctx.Done():
			return
		}
	}
}
