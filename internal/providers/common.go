package providers

import (
	"context"

	"github.com/IonBazan/gangplank/internal/types"
)

type PortProvider interface {
	GetPortMappings(ctx context.Context) ([]types.PortMapping, error)
}

// Listen blocks until ctx is cancelled. A nil channel disables that event kind.
type EventPortProvider interface {
	Listen(ctx context.Context, events PortEventChannels)
}

type PortEventChannels struct {
	Add    chan<- types.PortMapping
	Delete chan<- types.PortMapping
}
