package upnp

import (
	"context"
	"log"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/huin/goupnp/soap"
)

// DummyConnection is a mock UPnP client that echoes requests for testing.
type DummyConnection struct {
	Forwarded []types.PortMapping
	Deleted   []struct {
		ExtPort  uint16
		Protocol string
	}
	ForwardErr error
	DeleteErr  error
}

func (c *DummyConnection) GetExternalIPAddress() (string, error) {
	log.Println("[Dummy UPnP] External IP address requested - returning 203.0.113.1")
	return "203.0.113.1", nil
}

func (c *DummyConnection) AddPortMapping(
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

func (c *DummyConnection) DeletePortMapping(
	NewRemoteHost string,
	NewExternalPort uint16,
	NewProtocol string,
) (err error) {
	log.Printf("[Dummy UPnP] Deleting port mapping: ExternalPort=%d, Protocol=%s", NewExternalPort, NewProtocol)

	if c.DeleteErr != nil {
		return c.DeleteErr
	}

	c.Deleted = append(c.Deleted, struct {
		ExtPort  uint16
		Protocol string
	}{NewExternalPort, NewProtocol})
	return nil
}

func (c *DummyConnection) GetGenericPortMappingEntryCtx(
	ctx context.Context,
	NewPortMappingIndex uint16,
) (NewRemoteHost string, NewExternalPort uint16, NewProtocol string, NewInternalPort uint16, NewInternalClient string, NewEnabled bool, NewPortMappingDescription string, NewLeaseDuration uint32, err error) {
	log.Printf("[Dummy UPnP] Listing port mapping: Index=%d", NewPortMappingIndex)

	if NewPortMappingIndex == 0 {
		return "", 8080, "TCP", 80, "192.168.1.100", true, "Test Mapping", 3600, nil
	}

	return "", 0, "", 0, "", false, "", 0, NewSpecifiedArrayIndexInvalidError()
}

func NewSpecifiedArrayIndexInvalidError() error {
	return &soap.SOAPFaultError{
		FaultCode:   "s:Client",
		FaultString: "UPnPError",
		Detail: struct {
			UPnPError struct {
				Errorcode        int    `xml:"errorCode"`
				ErrorDescription string `xml:"errorDescription"`
			} `xml:"UPnPError"`
			Raw []byte `xml:",innerxml"`
		}{
			UPnPError: struct {
				Errorcode        int    `xml:"errorCode"`
				ErrorDescription string `xml:"errorDescription"`
			}{
				Errorcode:        714,
				ErrorDescription: "SpecifiedArrayIndexInvalid",
			},
			Raw: []byte(`<UPnPError><errorCode>714</errorCode><errorDescription>SpecifiedArrayIndexInvalid</errorDescription></UPnPError>`),
		},
	}
}
