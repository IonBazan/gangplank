package providers

import (
	"context"
	"fmt"
	"sort"

	"github.com/moby/moby/client"

	"github.com/IonBazan/gangplank/internal/portmap"
)

type ContainerLister interface {
	ContainerList(ctx context.Context, options client.ContainerListOptions) (client.ContainerListResult, error)
}

type DockerPortProvider struct {
	dockerCli ContainerLister
}

func NewDockerPortProvider(cli ContainerLister) *DockerPortProvider {
	return &DockerPortProvider{dockerCli: cli}
}

func (d *DockerPortProvider) GetPortMappings(ctx context.Context) ([]portmap.Mapping, error) {
	byContainer, err := listContainerMappings(ctx, d.dockerCli)
	if err != nil {
		return nil, err
	}

	// Sorted so that conflicts between containers always resolve the same way.
	var mappings []portmap.Mapping
	for _, ctr := range byContainer {
		mappings = append(mappings, ctr.mappings...)
	}
	sort.SliceStable(mappings, func(i, j int) bool {
		if mappings[i].Name != mappings[j].Name {
			return mappings[i].Name < mappings[j].Name
		}
		if mappings[i].ExternalPort != mappings[j].ExternalPort {
			return mappings[i].ExternalPort < mappings[j].ExternalPort
		}
		return mappings[i].Protocol < mappings[j].Protocol
	})

	return mappings, nil
}

type containerMappings struct {
	id       string
	mappings []portmap.Mapping
}

func listContainerMappings(ctx context.Context, cli ContainerLister) ([]containerMappings, error) {
	containers, err := cli.ContainerList(ctx, client.ContainerListOptions{
		Filters: make(client.Filters).Add("status", "running"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]containerMappings, 0, len(containers.Items))
	for _, ctr := range containers.Items {
		result = append(result, containerMappings{id: ctr.ID, mappings: extractPortsFromContainer(ctr)})
	}

	return result, nil
}
