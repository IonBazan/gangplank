package providers

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/IonBazan/gangplank/internal/types"
)

type ContainerInfo struct {
	Labels        map[string]string
	Ports         []container.PortSummary
	ContainerName string
	ID            string
}

// labelForward lists host ports to forward: "published", or "<external>:<host port>[/<protocol>]".
const labelForward = "gangplank.forward"

// labelForwardContainer lists container ports to forward, resolving the host port Docker bound them to:
// "published", or "[<external>:]<container port>[/<protocol>]".
const labelForwardContainer = "gangplank.forward.container"

func extractPortsFromContainer(ctr container.Summary) []types.PortMapping {
	var mappings []types.PortMapping
	containerName := shortID(ctr.ID)
	if len(ctr.Names) > 0 && ctr.Names[0] != "" {
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

	return dedupe(mappings)
}

func parseDockerLabel(label string, info ContainerInfo, isContainerRef bool) []types.PortMapping {
	var mappings []types.PortMapping

	for _, part := range strings.Split(label, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if part == "published" {
			for _, port := range info.Ports {
				if !isForwardable(port) {
					continue
				}
				// The router forwards to this host, so the internal port is the host port.
				mappings = append(mappings, types.PortMapping{
					ExternalPort: int(port.PublicPort),
					InternalPort: int(port.PublicPort),
					Protocol:     strings.ToUpper(port.Type),
					Name:         info.ContainerName,
				})
			}
			continue
		}

		if isContainerRef {
			mapping, err := resolveContainerPort(part, info.Ports)
			if err != nil {
				log.Printf("Invalid container port mapping %s for container %s: %v", part, shortID(info.ID), err)
				continue
			}
			mapping.Name = info.ContainerName
			mappings = append(mappings, mapping)
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

// When the external port is omitted, the container port number is used.
func resolveContainerPort(spec string, ports []container.PortSummary) (types.PortMapping, error) {
	mapping, err := types.ParsePortMapping(spec)
	if err != nil {
		return mapping, err
	}
	containerPort := mapping.InternalPort
	if !strings.Contains(spec, ":") {
		mapping.ExternalPort = containerPort
	}

	for _, port := range ports {
		if int(port.PrivatePort) == containerPort && strings.EqualFold(port.Type, mapping.Protocol) && isForwardable(port) {
			mapping.InternalPort = int(port.PublicPort)
			return mapping, nil
		}
	}

	return mapping, fmt.Errorf("container port %d/%s is not published", containerPort, mapping.Protocol)
}

func isForwardable(port container.PortSummary) bool {
	return port.PublicPort != 0 && !port.IP.IsLoopback()
}

// Docker reports each binding twice, for 0.0.0.0 and ::.
func dedupe(mappings []types.PortMapping) []types.PortMapping {
	seen := make(map[string]bool, len(mappings))
	result := mappings[:0]
	for _, m := range mappings {
		if seen[m.Key()] {
			continue
		}
		seen[m.Key()] = true
		result = append(result, m)
	}

	return result
}

func shortID(id string) string {
	const maxLen = 12
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

func portsFromInspect(info container.InspectResponse) []container.PortSummary {
	if info.NetworkSettings == nil {
		return nil
	}

	var ports []container.PortSummary
	for portProto, bindings := range info.NetworkSettings.Ports {
		for _, binding := range bindings {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil || hostPort <= 0 || hostPort > 65535 {
				continue
			}
			ports = append(ports, container.PortSummary{
				IP:          binding.HostIP,
				PrivatePort: portProto.Num(),
				PublicPort:  uint16(hostPort),
				Type:        string(portProto.Proto()),
			})
		}
	}

	return ports
}
