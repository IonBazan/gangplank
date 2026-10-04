package providers

import (
	"context"

	"github.com/IonBazan/gangplank/internal/portmap"
)

type PortProvider interface {
	GetPortMappings(ctx context.Context) ([]portmap.Mapping, error)
}

// Listen blocks until ctx is cancelled. A nil channel disables that event kind.
type EventPortProvider interface {
	Listen(ctx context.Context, events PortEventChannels)
}

type PortEventChannels struct {
	Add    chan<- portmap.Mapping
	Delete chan<- portmap.Mapping
}
