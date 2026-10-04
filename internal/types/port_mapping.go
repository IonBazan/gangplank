package types

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// PortMapping represents a single port mapping configuration.
type PortMapping struct {
	ExternalPort int    `mapstructure:"externalPort" yaml:"externalPort"`
	InternalPort int    `mapstructure:"internalPort" yaml:"internalPort"`
	Protocol     string `mapstructure:"protocol" yaml:"protocol"`
	Name         string `mapstructure:"name" yaml:"name"`
}

// Key uniquely identifies a mapping on the gateway (external port and protocol).
func (p PortMapping) Key() string {
	return fmt.Sprintf("%d/%s", p.ExternalPort, p.Protocol)
}

// Normalize upper-cases the protocol and defaults it to TCP when empty.
func (p PortMapping) Normalize() PortMapping {
	p.Protocol = strings.ToUpper(strings.TrimSpace(p.Protocol))
	if p.Protocol == "" {
		p.Protocol = "TCP"
	}

	return p
}

func (p PortMapping) Validate() error {
	if p.ExternalPort <= 0 || p.ExternalPort > 65535 {
		return fmt.Errorf("external port must be a number between 1 and 65535, got %d", p.ExternalPort)
	}
	if p.InternalPort <= 0 || p.InternalPort > 65535 {
		return fmt.Errorf("internal port must be a number between 1 and 65535, got %d", p.InternalPort)
	}
	protocol := strings.ToUpper(p.Protocol)
	if protocol != "TCP" && protocol != "UDP" {
		return fmt.Errorf("protocol must be 'TCP' or 'UDP', got %s", p.Protocol)
	}

	return nil
}

// ParsePortMapping parses a string in the format "<external>:<internal>/<protocol>", "<external>:<internal>", or "<port>".
// If no protocol is provided, it defaults to TCP.
// If a single port is provided, it is used for both external and internal ports.
func ParsePortMapping(mappingStr string) (PortMapping, error) {
	var mapping PortMapping

	parts := strings.Split(mappingStr, "/")
	protocol := "TCP" // Default to TCP if no protocol is provided
	if len(parts) == 2 {
		protocol = strings.ToUpper(parts[1])
	} else if len(parts) != 1 {
		return mapping, errors.New("invalid format: expected <external>:<internal>[/<protocol>] or <port>")
	}
	mapping.Protocol = protocol

	// Parse the port part (e.g., "8080:80", "8080", ":80", "80:")
	ports := strings.Split(parts[0], ":")
	var extPort, intPort int
	var err error

	switch len(ports) {
	case 1:
		// Single port provided (e.g., "8080")
		if ports[0] == "" {
			return mapping, errors.New("invalid port format: port cannot be empty")
		}
		if extPort, err = parsePort(ports[0], "port"); err != nil {
			return mapping, err
		}
		intPort = extPort
	case 2:
		// External and/or internal ports provided (e.g., "8080:80", ":80", "80:")
		extPortStr, intPortStr := ports[0], ports[1]

		if extPortStr == "" && intPortStr == "" {
			return mapping, errors.New("invalid port format: both external and internal ports cannot be empty")
		}

		if extPortStr != "" {
			if extPort, err = parsePort(extPortStr, "external port"); err != nil {
				return mapping, err
			}
		}

		if intPortStr != "" {
			if intPort, err = parsePort(intPortStr, "internal port"); err != nil {
				return mapping, err
			}
		}

		if extPortStr == "" {
			extPort = intPort
		}
		if intPortStr == "" {
			intPort = extPort
		}
	default:
		return mapping, errors.New("invalid port format: expected <external>:<internal> or <port>")
	}

	mapping.ExternalPort = extPort
	mapping.InternalPort = intPort

	if err := mapping.Validate(); err != nil {
		return mapping, err
	}

	return mapping, nil
}

func parsePort(s, field string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("%s must be a number between 1 and 65535, got %q", field, s)
	}

	return port, nil
}
