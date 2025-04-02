package types

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

// PortMapping represents a single port mapping configuration.
type PortMapping struct {
	ExternalPort int    `yaml:"externalPort"`
	InternalPort int    `yaml:"internalPort"`
	Protocol     string `yaml:"protocol"`
	Name         string `yaml:"name"`
}

// PortFetcher is for static port fetching.
type PortFetcher interface {
	FetchPorts() ([]PortMapping, error)
}

// EventPortFetcher is for event-driven port fetching using channels.
type EventPortFetcher interface {
	Listen(ctx context.Context, addCh chan<- PortMapping, deleteCh chan<- PortMapping)
}

func (p PortMapping) Validate() error {
	if p.ExternalPort <= 0 || p.ExternalPort > 65535 {
		return fmt.Errorf("externalPort must be between 1 and 65535, got %d", p.ExternalPort)
	}
	if p.InternalPort <= 0 || p.InternalPort > 65535 {
		return fmt.Errorf("internalPort must be between 1 and 65535, got %d", p.InternalPort)
	}
	protocol := strings.ToUpper(p.Protocol)
	if protocol != "TCP" && protocol != "UDP" {
		return fmt.Errorf("protocol must be 'TCP' or 'UDP', got %s", p.Protocol)
	}

	return nil
}

// ParsePortMapping parses a string in the format "<external>:<internal>/<protocol>" into a PortMapping.
func ParsePortMapping(mappingStr string) (PortMapping, error) {
	var mapping PortMapping

	parts := strings.Split(mappingStr, "/")
	if len(parts) != 2 {
		return mapping, logError("Invalid format: expected <external>:<internal>/<protocol>")
	}

	protocol := strings.ToUpper(parts[1])
	if protocol != "TCP" && protocol != "UDP" {
		return mapping, logError("Invalid protocol: must be TCP or UDP")
	}
	mapping.Protocol = protocol

	ports := strings.Split(parts[0], ":")
	if len(ports) != 2 {
		return mapping, logError("Invalid port format: expected <external>:<internal>")
	}

	extPort, err := strconv.Atoi(ports[0])
	if err != nil || extPort <= 0 || extPort > 65535 {
		return mapping, logError("Invalid external port: must be a number between 1 and 65535")
	}
	mapping.ExternalPort = extPort

	intPort, err := strconv.Atoi(ports[1])
	if err != nil || intPort <= 0 || intPort > 65535 {
		return mapping, logError("Invalid internal port: must be a number between 1 and 65535")
	}
	mapping.InternalPort = intPort

	return mapping, nil
}

func logError(msg string) error {
	log.Printf("Error parsing port mapping: %s", msg)
	return fmt.Errorf(msg)
}
