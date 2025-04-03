package providers

import (
	"context"
	dockerevents "github.com/docker/docker/api/types/events"
	"log"
	"strconv"
	"strings"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// EventInspector defines the minimal interface for DockerEventPortProvider.
type EventInspector interface {
	Events(ctx context.Context, options dockerevents.ListOptions) (<-chan dockerevents.Message, <-chan error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

type DockerEventPortProvider struct {
	dockerCli EventInspector
}

func NewDockerEventPortProvider(cli EventInspector) *DockerEventPortProvider {
	return &DockerEventPortProvider{dockerCli: cli}
}

func (d *DockerEventPortProvider) GetPortMappings() ([]types.PortMapping, error) {
	return nil, nil
}

func (d *DockerEventPortProvider) Listen(ctx context.Context, events PortEventChannels) {
	filterArgs := filters.NewArgs(
		filters.Arg("type", "container"),
		filters.Arg("event", "start"),
		filters.Arg("event", "stop"),
		filters.Arg("event", "die"),
	)
	eventChan, errChan := d.dockerCli.Events(ctx, dockerevents.ListOptions{
		Filters: filterArgs,
	})
	for {
		select {
		case event := <-eventChan:
			switch event.Action {
			case "start":
				if events.Add != nil {
					go d.handleContainerStart(event.Actor.ID, events.Add)
				}
			case "stop", "die":
				if events.Delete != nil {
					go d.handleContainerStop(event.Actor.ID, events.Delete)
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

func (d *DockerEventPortProvider) handleContainerStart(containerID string, addCh chan<- types.PortMapping) {
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

func (d *DockerEventPortProvider) handleContainerStop(containerID string, deleteCh chan<- types.PortMapping) {
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
