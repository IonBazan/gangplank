package fetchers

import (
	"context"
	"github.com/docker/docker/api/types/events"
	"log"
	"strconv"
	"strings"

	gangplanktypes "github.com/IonBazan/gangplank/internal/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// EventInspector defines the minimal interface for DockerEventPortFetcher.
type EventInspector interface {
	Events(ctx context.Context, options events.ListOptions) (<-chan events.Message, <-chan error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

type DockerEventPortFetcher struct {
	dockerCli EventInspector
}

func NewDockerEventPortFetcher(cli EventInspector) *DockerEventPortFetcher {
	return &DockerEventPortFetcher{dockerCli: cli}
}

func (d *DockerEventPortFetcher) FetchPorts() ([]gangplanktypes.PortMapping, error) {
	return nil, nil
}

func (d *DockerEventPortFetcher) Listen(ctx context.Context, addCh chan<- gangplanktypes.PortMapping, deleteCh chan<- gangplanktypes.PortMapping) {
	filterArgs := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("event", "start"),
		filters.Arg("event", "stop"),
		filters.Arg("event", "die"),
	)
	eventChan, errChan := d.dockerCli.Events(ctx, events.ListOptions{
		Filters: filterArgs,
	})
	for {
		select {
		case event := <-eventChan:
			switch event.Action {
			case "start":
				go d.handleContainerStart(event.Actor.ID, addCh)
			case "stop", "die":
				if deleteCh != nil {
					go d.handleContainerStop(event.Actor.ID, deleteCh)
				}
			}
		case err := <-errChan:
			if err != nil {
				log.Printf("Error receiving Docker events: %v", err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (d *DockerEventPortFetcher) handleContainerStart(containerID string, addCh chan<- gangplanktypes.PortMapping) {
	info, err := d.dockerCli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		log.Printf("Failed to inspect container %s: %v", containerID[:12], err)
		return
	}

	var ports []container.Port
	for portProto, bindings := range info.NetworkSettings.Ports {
		for _, binding := range bindings {
			if binding.HostPort == "" {
				continue
			}
			extPort, _ := strconv.Atoi(binding.HostPort)
			ports = append(ports, container.Port{
				PrivatePort: uint16(portProto.Int()),
				PublicPort:  uint16(extPort),
				Type:        portProto.Proto(),
			})
		}
	}

	ctr := container.Summary{
		ID:     info.ID,
		Labels: info.Config.Labels,
		Ports:  ports,
	}
	mappings := extractPortsFromContainer(ctr)
	for _, m := range mappings {
		if info.Name != "" {
			m.Name = strings.TrimPrefix(info.Name, "/")
		} else {
			m.Name = containerID[:12]
		}
		addCh <- m
	}
}

func (d *DockerEventPortFetcher) handleContainerStop(containerID string, deleteCh chan<- gangplanktypes.PortMapping) {
	info, err := d.dockerCli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		log.Printf("Failed to inspect container %s: %v", containerID[:12], err)
		return
	}

	var ports []container.Port
	for portProto, bindings := range info.NetworkSettings.Ports {
		for _, binding := range bindings {
			if binding.HostPort == "" {
				continue
			}
			extPort, _ := strconv.Atoi(binding.HostPort)
			ports = append(ports, container.Port{
				PrivatePort: uint16(portProto.Int()),
				PublicPort:  uint16(extPort),
				Type:        portProto.Proto(),
			})
		}
	}

	ctr := container.Summary{
		ID:     info.ID,
		Labels: info.Config.Labels,
		Ports:  ports,
	}
	mappings := extractPortsFromContainer(ctr)
	for _, m := range mappings {
		if info.Name != "" {
			m.Name = strings.TrimPrefix(info.Name, "/")
		} else {
			m.Name = containerID[:12]
		}
		deleteCh <- m
	}
}
