package providers

import (
	"context"
	"github.com/IonBazan/gangplank/internal/types"
)

// PortProvider is for static port mapping retrieval.
type PortProvider interface {
	GetPortMappings() ([]types.PortMapping, error)
}

// EventPortProvider is for event-driven port mapping retrieval using channels.
type EventPortProvider interface {
	Listen(ctx context.Context, events PortEventChannels)
}

type PortEventChannels struct {
	Add    chan<- types.PortMapping
	Delete chan<- types.PortMapping
}
