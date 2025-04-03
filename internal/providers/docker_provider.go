package providers

import (
	"context"
	"fmt"

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

func (d *DockerPortProvider) GetPortMappings() ([]types.PortMapping, error) {
	containers, err := d.dockerCli.ContainerList(context.Background(), container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("status", "running")),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %v", err)
	}

	var mappings []types.PortMapping
	for _, ctr := range containers {
		mappings = append(mappings, extractPortsFromContainer(ctr)...)
	}
	return mappings, nil
}
