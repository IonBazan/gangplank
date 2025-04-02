package upnp

import (
	"log"
)

// DummyConnection is a mock UPnP client that echoes requests for testing.
type DummyConnection struct {
}

func (c *DummyConnection) GetExternalIPAddress() (string, error) {
	log.Println("Dummy UPnP: GetExternalIPAddress called, returning 203.0.113.1")
	return "203.0.113.1", nil
}

func (c *DummyConnection) AddPortMapping(NewRemoteHost string, NewExternalPort uint16, NewProtocol string, NewInternalPort uint16, NewInternalClient string, NewEnabled bool, NewPortMappingDescription string, NewLeaseDuration uint32) (err error) {
	log.Printf("Dummy UPnP: AddPortMapping called - ExternalPort=%d, InternalPort=%d, Protocol=%s, InternalIP=%s, Description=%s TTL=%d",
		NewExternalPort, NewInternalPort, NewProtocol, NewInternalClient, NewPortMappingDescription, NewLeaseDuration)
	return nil
}

func (c *DummyConnection) DeletePortMapping(NewRemoteHost string, NewExternalPort uint16, NewProtocol string) (err error) {
	log.Printf("Dummy UPnP: DeletePortMapping called - ExternalPort=%d, Protocol=%s", NewExternalPort, NewProtocol)
	return nil
}
