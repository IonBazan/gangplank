package upnptest

import (
	"context"
	"sync"

	"github.com/huin/goupnp/soap"

	"github.com/IonBazan/gangplank/internal/portmap"
)

type Deleted struct {
	ExtPort  uint16
	Protocol string
}

// Connection is an in-memory UPnP connection for tests. Read results with Snapshot.
type Connection struct {
	mu        sync.Mutex
	Forwarded []portmap.Mapping
	Deleted   []Deleted
	// Existing is what the gateway lists. Lease is reported as the lease duration.
	Existing      []Mapping
	ForwardErr    error
	DeleteErr     error
	ExternalIPErr error
}

func (c *Connection) Snapshot() ([]portmap.Mapping, []Deleted) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]portmap.Mapping(nil), c.Forwarded...), append([]Deleted(nil), c.Deleted...)
}

func (c *Connection) GetExternalIPAddressCtx(context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ExternalIPErr != nil {
		return "", c.ExternalIPErr
	}
	return "203.0.113.1", nil
}

func (c *Connection) AddPortMappingCtx(_ context.Context, _ string, ext uint16, protocol string, internal uint16, _ string, _ bool, description string, _ uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ForwardErr != nil {
		return c.ForwardErr
	}
	c.Forwarded = append(c.Forwarded, portmap.Mapping{ExternalPort: int(ext), InternalPort: int(internal), Protocol: protocol, Name: description})
	return nil
}

func (c *Connection) DeletePortMappingCtx(_ context.Context, _ string, ext uint16, protocol string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.DeleteErr != nil {
		return c.DeleteErr
	}
	c.Deleted = append(c.Deleted, Deleted{ext, protocol})
	return nil
}

func (c *Connection) GetGenericPortMappingEntryCtx(_ context.Context, index uint16) (string, uint16, string, uint16, string, bool, string, uint32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if int(index) >= len(c.Existing) {
		return "", 0, "", 0, "", false, "", 0, UPnPError(713, "SpecifiedArrayIndexInvalid")
	}
	e := c.Existing[index]
	return "", uint16(e.ExternalPort), e.Protocol, uint16(e.InternalPort), e.InternalIP, true, e.Description, uint32(e.Lease), nil
}

// UPnPError builds the SOAP fault a gateway returns for the given UPnP error code.
func UPnPError(code int, description string) error {
	serr := &soap.SOAPFaultError{FaultCode: "s:Client", FaultString: "UPnPError"}
	serr.Detail.UPnPError.Errorcode = code
	serr.Detail.UPnPError.ErrorDescription = description

	return serr
}
