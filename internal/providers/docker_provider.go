package providers

import (
	"context"
	"fmt"
	"sort"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

type ContainerLister interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
}

type DockerPortProvider struct {
	dockerCli ContainerLister
}

func NewDockerPortProvider(cli ContainerLister) *DockerPortProvider {
	return &DockerPortProvider{dockerCli: cli}
}

func (d *DockerPortProvider) GetPortMappings(ctx context.Context) ([]types.PortMapping, error) {
	byContainer, err := listContainerMappings(ctx, d.dockerCli)
	if err != nil {
		return nil, err
	}

	// Sort by container name so conflicts between containers resolve deterministically.
	var mappings []types.PortMapping
	for _, ctr := range byContainer {
		mappings = append(mappings, ctr.mappings...)
	}
	sort.SliceStable(mappings, func(i, j int) bool {
		if mappings[i].Name != mappings[j].Name {
			return mappings[i].Name < mappings[j].Name
		}
		return mappings[i].Key() < mappings[j].Key()
	})

	return mappings, nil
}

type containerMappings struct {
	id       string
	mappings []types.PortMapping
}

func listContainerMappings(ctx context.Context, cli ContainerLister) ([]containerMappings, error) {
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("status", "running")),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]containerMappings, 0, len(containers))
	for _, ctr := range containers {
		result = append(result, containerMappings{id: ctr.ID, mappings: extractPortsFromContainer(ctr)})
	}

	return result, nil
}
