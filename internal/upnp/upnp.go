package upnp

import (
	"context"
	"fmt"
	"github.com/huin/goupnp/soap"
	"log"
	"net"
	"net/url"
	"time"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/huin/goupnp/dcps/internetgateway2"
)

const DefaultLeaseDuration = 60 * time.Minute
const defaultDescription = "Gangplank UPnP"

type UPnPConnection interface {
	AddPortMapping(
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
		NewInternalPort uint16,
		NewInternalClient string,
		NewEnabled bool,
		NewPortMappingDescription string,
		NewLeaseDuration uint32,
	) (err error)

	DeletePortMapping(
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
	) (err error)

	GetExternalIPAddress() (
		NewExternalIPAddress string,
		err error,
	)

	GetGenericPortMappingEntryCtx(
		ctx context.Context,
		NewPortMappingIndex uint16,
	) (NewRemoteHost string, NewExternalPort uint16, NewProtocol string, NewInternalPort uint16, NewInternalClient string, NewEnabled bool, NewPortMappingDescription string, NewLeaseDuration uint32, err error)
}

type PortMappingEntry struct {
	ExternalPort  int
	InternalPort  int
	Protocol      string
	InternalIP    string
	Description   string
	LeaseDuration uint32
	Enabled       bool
}

// Client wraps the UPnP client and local IP for port forwarding.
type Client struct {
	uPnPConnection UPnPConnection
	LocalIP        string
	duration       time.Duration
}

func NewClient(localIPOverride, gatewayOverride string, duration time.Duration) (*Client, error) {
	var upnpClient UPnPConnection
	var err error

	if gatewayOverride != "" {
		upnpClient, err = clientFromGateway(gatewayOverride)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		upnpClient, err = discoverGateway(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize UPnP client: %v", err)
	}

	localIP := localIPOverride
	if localIP == "" {
		localIP, err = getLocalIP()
		if err != nil {
			return nil, fmt.Errorf("failed to determine local IP: %v", err)
		}
	}

	return NewClientWithConnection(upnpClient, localIP, duration), nil
}

func NewClientWithConnection(connection UPnPConnection, localIP string, duration time.Duration) *Client {
	return &Client{
		uPnPConnection: connection,
		LocalIP:        localIP,
		duration:       duration,
	}
}

func NewDummyClient(duration time.Duration) *Client {
	return NewClientWithConnection(&DummyConnection{}, "192.168.1.100", duration)
}

func (u *Client) ForwardPorts(mappings []types.PortMapping) error {
	for _, m := range mappings {
		err := u.addPortMapping(m)
		if err != nil {
			log.Printf("Failed to forward port %d/%s for %s: %v", m.ExternalPort, m.Protocol, m.Name, err)

			return err
		} else {
			log.Printf("Successfully forwarded port %d/%s for %s", m.ExternalPort, m.Protocol, m.Name)
		}
	}
	return nil
}

func (u *Client) addPortMapping(m types.PortMapping) error {
	description := defaultDescription
	if m.Name != "" {
		description = fmt.Sprintf("%s: %s", defaultDescription, m.Name)
	}
	return u.uPnPConnection.AddPortMapping(
		"",
		uint16(m.ExternalPort),
		m.Protocol,
		uint16(m.InternalPort),
		u.LocalIP,
		true,
		description,
		uint32(u.duration.Seconds()),
	)
}

func (u *Client) DeletePortMapping(externalPort int, protocol string) error {
	return u.uPnPConnection.DeletePortMapping("", uint16(externalPort), protocol)
}

// ListPortMappings retrieves all active UPnP port mappings.
func (c *Client) ListPortMappings() ([]PortMappingEntry, error) {
	var mappings []PortMappingEntry
	index := uint16(0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		_, externalPort, protocol, internalPort, internalClient, enabled, description, leaseDuration, err := c.uPnPConnection.GetGenericPortMappingEntryCtx(ctx, index)
		if err != nil {
			if serr, ok := err.(*soap.SOAPFaultError); ok && serr.Detail.UPnPError.ErrorDescription == "SpecifiedArrayIndexInvalid" {
				break
			}

			return nil, fmt.Errorf("failed to get port mapping at index %d: %v", index, err)
		}

		mappings = append(mappings, PortMappingEntry{
			ExternalPort:  int(externalPort),
			InternalPort:  int(internalPort),
			Protocol:      protocol,
			InternalIP:    internalClient,
			Description:   description,
			LeaseDuration: leaseDuration,
			Enabled:       enabled,
		})

		index++
	}

	return mappings, nil
}

func discoverGateway(ctx context.Context) (UPnPConnection, error) {
	if clients, _, err := internetgateway2.NewWANIPConnection2Clients(); err == nil && len(clients) > 0 {
		return clients[0], nil
	}
	if clients, _, err := internetgateway1.NewWANIPConnection1Clients(); err == nil && len(clients) > 0 {
		return clients[0], nil
	}
	return nil, fmt.Errorf("no UPnP IGD found within timeout")
}

func clientFromGateway(gatewayURL string) (UPnPConnection, error) {
	location, err := url.Parse(gatewayURL)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway URL: %v", err)
	}
	if igd2Clients, err := internetgateway2.NewWANIPConnection2ClientsByURL(location); err == nil && len(igd2Clients) > 0 {
		return igd2Clients[0], nil
	}
	if igd1Clients, err := internetgateway1.NewWANIPConnection1ClientsByURL(location); err == nil && len(igd1Clients) > 0 {
		return igd1Clients[0], nil
	}
	return nil, fmt.Errorf("no supported UPnP service found at %s", gatewayURL)
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %v", err)
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no valid local IP found")
}
