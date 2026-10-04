package providers

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/moby/moby/api/types/container"
	dockerevents "github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"

	"github.com/IonBazan/gangplank/internal/types"
)

const (
	defaultRetryDelay = time.Second
	maxRetryDelay     = 30 * time.Second
)

type EventInspector interface {
	ContainerLister
	Events(ctx context.Context, options client.EventsListOptions) client.EventsResult
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

// DockerEventPortProvider remembers what each container exposed when it started,
// because Docker clears port bindings once a container stops.
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

// After a reconnect, running containers are resynced so that events missed
// while disconnected are not lost.
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
	stream := d.dockerCli.Events(streamCtx, client.EventsListOptions{
		Filters: make(client.Filters).
			Add("type", string(dockerevents.ContainerEventType)).
			Add("event", string(dockerevents.ActionStart), string(dockerevents.ActionStop), string(dockerevents.ActionDie)),
	})

	if err := d.sync(ctx, events, resync); err != nil {
		return false, err
	}

	for {
		select {
		case event, ok := <-stream.Messages:
			if !ok {
				return true, errors.New("event stream closed")
			}
			switch event.Action {
			case dockerevents.ActionStart:
				d.handleContainerStart(ctx, event.Actor.ID, events.Add)
			case dockerevents.ActionStop, dockerevents.ActionDie:
				d.handleContainerStop(ctx, event.Actor.ID, events.Delete)
			}
		case err := <-stream.Err:
			if err == nil {
				err = errors.New("event stream closed")
			}
			return true, err
		case <-ctx.Done():
			return true, nil
		}
	}
}

// With emit, containers that started or stopped since the previous sync are reported.
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
	result, err := d.dockerCli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		log.Printf("Failed to inspect container %s: %v", shortID(containerID), err)
		return
	}
	info := result.Container

	var labels map[string]string
	if info.Config != nil {
		labels = info.Config.Labels
	}

	mappings := extractPortsFromContainer(container.Summary{
		ID:     containerID,
		Names:  []string{info.Name},
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
