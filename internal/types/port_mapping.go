package types

import (
	"errors"
	"fmt"
	"log"
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

func (p PortMapping) Validate() error {
	if p.ExternalPort <= 0 || p.ExternalPort > 65535 {
		return fmt.Errorf("External port must be a number between 1 and 65535, got %d", p.ExternalPort)
	}
	if p.InternalPort <= 0 || p.InternalPort > 65535 {
		return fmt.Errorf("Internal port must be a number between 1 and 65535, got %d", p.InternalPort)
	}
	protocol := strings.ToUpper(p.Protocol)
	if protocol != "TCP" && protocol != "UDP" {
		return fmt.Errorf("Protocol must be 'TCP' or 'UDP', got %s", p.Protocol)
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
		return mapping, logError("Invalid format: expected <external>:<internal>[/<protocol>] or <port>")
	}
	mapping.Protocol = protocol

	// Parse the port part (e.g., "8080:80", "8080", ":80", "80:")
	portPart := parts[0]
	ports := strings.Split(portPart, ":")
	var extPort, intPort int

	switch len(ports) {
	case 1:
		// Single port provided (e.g., "8080")
		portStr := ports[0]
		if portStr == "" {
			return mapping, logError("Invalid port format: port cannot be empty")
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return mapping, logError("Invalid port: must be a number between 1 and 65535")
		}
		extPort = port
		intPort = port
	case 2:
		// External and/or internal ports provided (e.g., "8080:80", ":80", "80:")
		extPortStr, intPortStr := ports[0], ports[1]

		if extPortStr == "" && intPortStr == "" {
			return mapping, logError("Invalid port format: both external and internal ports cannot be empty")
		}

		if extPortStr != "" {
			extPort, _ = strconv.Atoi(extPortStr)
		}

		if intPortStr != "" {
			intPort, _ = strconv.Atoi(intPortStr)
		}

		if extPortStr == "" {
			extPort = intPort
		}
		if intPortStr == "" {
			intPort = extPort
		}
	default:
		return mapping, logError("Invalid port format: expected <external>:<internal> or <port>")
	}

	mapping.ExternalPort = extPort
	mapping.InternalPort = intPort

	err := mapping.Validate()

	if err != nil {
		return mapping, err
	}

	return mapping, nil
}

func logError(msg string) error {
	log.Printf("Error parsing port mapping: %s", msg)
	return errors.New(msg)
}
