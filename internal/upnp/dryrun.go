package upnp

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/huin/goupnp/soap"
)

// NewDryRunClient returns a client that only logs what it would do.
func NewDryRunClient(duration time.Duration) *Client {
	localIP, err := firstInterfaceIP()
	if err != nil {
		localIP = "0.0.0.0"
	}

	return NewClientWithConnection(dryRunConnection{}, localIP, duration)
}

type dryRunConnection struct{}

var endOfList = func() error {
	serr := &soap.SOAPFaultError{}
	serr.Detail.UPnPError.Errorcode = errCodeSpecifiedArrayIndexInvalid
	return serr
}()

func (dryRunConnection) GetExternalIPAddressCtx(context.Context) (string, error) {
	return "", nil
}

func (dryRunConnection) AddPortMappingCtx(_ context.Context, _ string, ext uint16, protocol string, internal uint16, client string, _ bool, description string, lease uint32) error {
	slog.Info("Dry run: would add port mapping", "port", fmt.Sprintf("%d/%s", ext, protocol), "target", fmt.Sprintf("%s:%d", client, internal), "description", description, "lease_seconds", lease)
	return nil
}

func (dryRunConnection) DeletePortMappingCtx(_ context.Context, _ string, ext uint16, protocol string) error {
	slog.Info("Dry run: would delete port mapping", "port", fmt.Sprintf("%d/%s", ext, protocol))
	return nil
}

func (dryRunConnection) GetGenericPortMappingEntryCtx(context.Context, uint16) (string, uint16, string, uint16, string, bool, string, uint32, error) {
	return "", 0, "", 0, "", false, "", 0, endOfList
}
