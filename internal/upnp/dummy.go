package upnp

import (
	"context"
	"log"
	"sync"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/huin/goupnp/soap"
)

type DeletedMapping struct {
	ExtPort  uint16
	Protocol string
}

// DummyConnection is a mock UPnP client that echoes requests for testing.
// It is safe for concurrent use; read results with Snapshot.
type DummyConnection struct {
	mu        sync.Mutex
	Forwarded []types.PortMapping
	Deleted   []DeletedMapping
	// Existing, when non-nil, is returned by GetGenericPortMappingEntryCtx instead of the default sample entry.
	Existing   []PortMappingEntry
	ForwardErr error
	DeleteErr  error
}

// Snapshot returns copies of the recorded forwarded and deleted mappings.
func (c *DummyConnection) Snapshot() ([]types.PortMapping, []DeletedMapping) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]types.PortMapping(nil), c.Forwarded...), append([]DeletedMapping(nil), c.Deleted...)
}

func (c *DummyConnection) GetExternalIPAddressCtx(ctx context.Context) (string, error) {
	log.Println("[Dummy UPnP] External IP address requested - returning 203.0.113.1")
	return "203.0.113.1", nil
}

func (c *DummyConnection) AddPortMappingCtx(
	ctx context.Context,
	NewRemoteHost string,
	NewExternalPort uint16,
	NewProtocol string,
	NewInternalPort uint16,
	NewInternalClient string,
	NewEnabled bool,
	NewPortMappingDescription string,
	NewLeaseDuration uint32,
) (err error) {
	log.Printf("[Dummy UPnP] Adding port mapping: ExternalPort=%d, InternalPort=%d, Protocol=%s, InternalIP=%s, Description=%s TTL=%d",
		NewExternalPort, NewInternalPort, NewProtocol, NewInternalClient, NewPortMappingDescription, NewLeaseDuration)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ForwardErr != nil {
		return c.ForwardErr
	}

	c.Forwarded = append(c.Forwarded, types.PortMapping{
		ExternalPort: int(NewExternalPort),
		InternalPort: int(NewInternalPort),
		Protocol:     NewProtocol,
		Name:         NewPortMappingDescription,
	})
	return nil
}

func (c *DummyConnection) DeletePortMappingCtx(
	ctx context.Context,
	NewRemoteHost string,
	NewExternalPort uint16,
	NewProtocol string,
) (err error) {
	log.Printf("[Dummy UPnP] Deleting port mapping: ExternalPort=%d, Protocol=%s", NewExternalPort, NewProtocol)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.DeleteErr != nil {
		return c.DeleteErr
	}

	c.Deleted = append(c.Deleted, DeletedMapping{NewExternalPort, NewProtocol})
	return nil
}

func (c *DummyConnection) GetGenericPortMappingEntryCtx(
	ctx context.Context,
	NewPortMappingIndex uint16,
) (NewRemoteHost string, NewExternalPort uint16, NewProtocol string, NewInternalPort uint16, NewInternalClient string, NewEnabled bool, NewPortMappingDescription string, NewLeaseDuration uint32, err error) {
	log.Printf("[Dummy UPnP] Listing port mapping: Index=%d", NewPortMappingIndex)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Existing != nil {
		if int(NewPortMappingIndex) < len(c.Existing) {
			e := c.Existing[NewPortMappingIndex]
			return "", uint16(e.ExternalPort), e.Protocol, uint16(e.InternalPort), e.InternalIP, e.Enabled, e.Description, e.LeaseDuration, nil
		}
	} else if NewPortMappingIndex == 0 {
		return "", 8080, "TCP", 80, "192.168.1.100", true, "Test Mapping", 3600, nil
	}

	return "", 0, "", 0, "", false, "", 0, NewUPnPError(errCodeSpecifiedArrayIndexInvalid, "SpecifiedArrayIndexInvalid")
}

// NewUPnPError builds a SOAP fault carrying the given UPnP error code.
func NewUPnPError(code int, description string) error {
	serr := &soap.SOAPFaultError{FaultCode: "s:Client", FaultString: "UPnPError"}
	serr.Detail.UPnPError.Errorcode = code
	serr.Detail.UPnPError.ErrorDescription = description

	return serr
}
