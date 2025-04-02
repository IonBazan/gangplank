package fetchers

import (
	"log"
	"strconv"
	"strings"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/docker/docker/api/types/container"
)

type ContainerInfo struct {
	Labels        map[string]string
	Ports         []container.Port
	ContainerName string
	ID            string
}

const labelForward = "gangplank.forward"
const labelForwardContainer = "gangplank.forward.container"

func extractPortsFromContainer(ctr container.Summary) []types.PortMapping {
	var mappings []types.PortMapping
	containerName := shortID(ctr.ID)
	if len(ctr.Names) > 0 {
		containerName = strings.TrimPrefix(ctr.Names[0], "/")
	}
	info := ContainerInfo{
		Labels:        ctr.Labels,
		Ports:         ctr.Ports,
		ContainerName: containerName,
		ID:            ctr.ID,
	}

	if val, ok := ctr.Labels[labelForward]; ok {
		mappings = append(mappings, parseDockerLabel(val, info, false)...)
	}
	if val, ok := ctr.Labels[labelForwardContainer]; ok {
		mappings = append(mappings, parseDockerLabel(val, info, true)...)
	}

	return mappings
}

func parseDockerLabel(label string, info ContainerInfo, isContainerRef bool) []types.PortMapping {
	var mappings []types.PortMapping
	parts := strings.Split(label, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "published" {
			for _, port := range info.Ports {
				if port.PublicPort == 0 {
					continue
				}
				mappings = append(mappings, types.PortMapping{
					ExternalPort: int(port.PublicPort),
					InternalPort: int(port.PrivatePort),
					Protocol:     strings.ToUpper(port.Type),
					Name:         info.ContainerName,
				})
			}
			continue
		}

		if isContainerRef {
			fields := strings.Split(part, "/")
			protocol := "TCP"
			if len(fields) == 2 && fields[1] != "" {
				upperProtocol := strings.ToUpper(fields[1])
				if upperProtocol == "TCP" || upperProtocol == "UDP" {
					protocol = upperProtocol
				}
			}
			intPort, err := strconv.Atoi(fields[0])
			if err != nil {
				log.Printf("Invalid container port %s for container %s: %v", fields[0], shortID(info.ID), err)
				continue
			}
			for _, port := range info.Ports {
				if int(port.PrivatePort) == intPort && port.PublicPort != 0 {
					mappings = append(mappings, types.PortMapping{
						ExternalPort: int(port.PublicPort),
						InternalPort: intPort,
						Protocol:     protocol,
						Name:         info.ContainerName,
					})
					break
				}
			}
		} else {
			mapping, err := types.ParsePortMapping(part)
			if err != nil {
				log.Printf("Invalid port mapping %s for container %s: %v", part, shortID(info.ID), err)
				continue
			}
			mapping.Name = info.ContainerName
			mappings = append(mappings, mapping)
		}
	}
	return mappings
}

func shortID(id string) string {
	const maxLen = 12
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}
