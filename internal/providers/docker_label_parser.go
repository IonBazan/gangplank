package providers

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/moby/moby/api/types/container"

	"github.com/IonBazan/gangplank/internal/portmap"
)

type containerInfo struct {
	name  string
	id    string
	ports []container.PortSummary
}

// labelForward lists host ports to forward: "published", or "<external>:<host port>[/<protocol>]".
const labelForward = "gangplank.forward"

// labelForwardContainer lists container ports to forward, resolving the host port Docker bound them to:
// "published", or "[<external>:]<container port>[/<protocol>]".
const labelForwardContainer = "gangplank.forward.container"

func extractPortsFromContainer(ctr container.Summary) []portmap.Mapping {
	info := containerInfo{name: shortID(ctr.ID), id: ctr.ID, ports: ctr.Ports}
	if len(ctr.Names) > 0 && ctr.Names[0] != "" {
		info.name = strings.TrimPrefix(ctr.Names[0], "/")
	}

	var mappings []portmap.Mapping
	if label, ok := ctr.Labels[labelForward]; ok {
		mappings = append(mappings, parseLabel(label, info, portmap.Parse)...)
	}
	if label, ok := ctr.Labels[labelForwardContainer]; ok {
		mappings = append(mappings, parseLabel(label, info, func(spec string) (portmap.Mapping, error) {
			return resolveContainerPort(spec, info.ports)
		})...)
	}

	return dedupe(mappings)
}

// parseLabel splits a comma-separated label and turns each entry into a mapping with parse.
func parseLabel(label string, info containerInfo, parse func(spec string) (portmap.Mapping, error)) []portmap.Mapping {
	var mappings []portmap.Mapping

	for _, part := range strings.Split(label, ",") {
		part = strings.TrimSpace(part)
		switch part {
		case "":
			continue
		case "published":
			mappings = append(mappings, publishedPorts(info)...)
			continue
		}

		mapping, err := parse(part)
		if err != nil {
			slog.Warn("Invalid port mapping in label", "entry", part, "container", info.name, "error", err)
			continue
		}
		mapping.Name = info.name
		mappings = append(mappings, mapping)
	}

	return mappings
}

// The router forwards to this host, so the internal port is the host port.
func publishedPorts(info containerInfo) []portmap.Mapping {
	var mappings []portmap.Mapping
	for _, port := range info.ports {
		if !isForwardable(port) {
			continue
		}
		mappings = append(mappings, portmap.Mapping{
			ExternalPort: int(port.PublicPort),
			InternalPort: int(port.PublicPort),
			Protocol:     strings.ToUpper(port.Type),
			Name:         info.name,
		})
	}

	return mappings
}

// When the external port is omitted, the container port number is used.
func resolveContainerPort(spec string, ports []container.PortSummary) (portmap.Mapping, error) {
	mapping, err := portmap.Parse(spec)
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
func dedupe(mappings []portmap.Mapping) []portmap.Mapping {
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
