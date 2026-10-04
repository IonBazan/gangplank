package upnp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/huin/goupnp/soap"

	"github.com/IonBazan/gangplank/internal/portmap"
)

const DefaultLeaseDuration = 60 * time.Minute

const DescriptionPrefix = "Gangplank UPnP"

const (
	discoveryTimeout = 5 * time.Second
	callTimeout      = 10 * time.Second
)

// From the UPnP IGD WANIPConnection spec.
const (
	errCodeSpecifiedArrayIndexInvalid   = 713
	errCodeNoSuchEntryInArray           = 714
	errCodeOnlyPermanentLeasesSupported = 725
)

type UPnPConnection interface {
	AddPortMappingCtx(
		ctx context.Context,
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
		NewInternalPort uint16,
		NewInternalClient string,
		NewEnabled bool,
		NewPortMappingDescription string,
		NewLeaseDuration uint32,
	) (err error)

	DeletePortMappingCtx(
		ctx context.Context,
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
	) (err error)

	GetExternalIPAddressCtx(ctx context.Context) (
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

func (e PortMappingEntry) IsOwned() bool {
	return strings.HasPrefix(e.Description, DescriptionPrefix)
}

type Client struct {
	uPnPConnection UPnPConnection
	LocalIP        string
	duration       time.Duration
	permanentOnly  atomic.Bool
}

func NewClient(ctx context.Context, localIPOverride, gatewayOverride string, duration time.Duration) (*Client, error) {
	var gw *gateway
	var err error

	if gatewayOverride != "" {
		gw, err = clientFromGateway(ctx, gatewayOverride)
	} else {
		discoverCtx, cancel := context.WithTimeout(ctx, discoveryTimeout)
		defer cancel()
		gw, err = discoverGateway(discoverCtx)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to initialize UPnP client: %w", err)
	}

	localIP := localIPOverride
	if localIP == "" {
		localIP, err = getLocalIP(gw.location, gw.localAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to determine local IP: %w", err)
		}
	}

	return NewClientWithConnection(gw.conn, localIP, duration), nil
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

func (u *Client) InternalIP() string {
	return u.LocalIP
}

func Description(name string) string {
	if name == "" {
		return DescriptionPrefix
	}

	return fmt.Sprintf("%s: %s", DescriptionPrefix, name)
}

// ForwardPorts keeps going after a failure and returns all errors joined.
func (u *Client) ForwardPorts(ctx context.Context, mappings []portmap.Mapping) error {
	var errs []error
	for _, m := range mappings {
		if err := u.addPortMapping(ctx, m.Normalize()); err != nil {
			log.Printf("Failed to forward port %d/%s for %s: %v", m.ExternalPort, m.Protocol, m.Name, err)
			errs = append(errs, fmt.Errorf("forward %d/%s: %w", m.ExternalPort, m.Protocol, err))
		} else {
			log.Printf("Successfully forwarded port %d/%s for %s", m.ExternalPort, m.Protocol, m.Name)
		}
	}

	return errors.Join(errs...)
}

func (u *Client) addPortMapping(ctx context.Context, m portmap.Mapping) error {
	lease := uint32(0)
	if !u.permanentOnly.Load() {
		lease = uint32(max(0, min(u.duration.Seconds(), math.MaxUint32)))
	}

	err := u.add(ctx, m, lease)
	if lease != 0 && hasErrorCode(err, errCodeOnlyPermanentLeasesSupported) {
		log.Printf("Gateway only supports permanent leases, retrying without lease duration")
		u.permanentOnly.Store(true)
		err = u.add(ctx, m, 0)
	}

	return err
}

func (u *Client) add(ctx context.Context, m portmap.Mapping, lease uint32) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	return u.uPnPConnection.AddPortMappingCtx(
		ctx,
		"",
		uint16(m.ExternalPort),
		m.Protocol,
		uint16(m.InternalPort),
		u.LocalIP,
		true,
		Description(m.Name),
		lease,
	)
}

func (u *Client) DeletePortMapping(ctx context.Context, externalPort int, protocol string) error {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	return u.uPnPConnection.DeletePortMappingCtx(ctx, "", uint16(externalPort), strings.ToUpper(protocol))
}

func (u *Client) GetExternalIP(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()

	return u.uPnPConnection.GetExternalIPAddressCtx(ctx)
}

func (u *Client) ListPortMappings(ctx context.Context) ([]PortMappingEntry, error) {
	var mappings []PortMappingEntry

	for index := 0; index <= math.MaxUint16; index++ {
		callCtx, cancel := context.WithTimeout(ctx, callTimeout)
		_, externalPort, protocol, internalPort, internalClient, enabled, description, leaseDuration, err := u.uPnPConnection.GetGenericPortMappingEntryCtx(callCtx, uint16(index))
		cancel()
		if err != nil {
			if isEndOfList(err) {
				break
			}

			return nil, fmt.Errorf("failed to get port mapping at index %d: %w", index, err)
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
	}

	return mappings, nil
}

// Gateways differ: most return 713, some 714, and a few only set the description.
func isEndOfList(err error) bool {
	var serr *soap.SOAPFaultError
	if !errors.As(err, &serr) {
		return false
	}

	code := serr.Detail.UPnPError.Errorcode
	desc := serr.Detail.UPnPError.ErrorDescription

	return code == errCodeSpecifiedArrayIndexInvalid ||
		code == errCodeNoSuchEntryInArray ||
		desc == "SpecifiedArrayIndexInvalid" ||
		desc == "NoSuchEntryInArray"
}

func hasErrorCode(err error, code int) bool {
	var serr *soap.SOAPFaultError
	return errors.As(err, &serr) && serr.Detail.UPnPError.Errorcode == code
}

type gateway struct {
	conn      UPnPConnection
	location  *url.URL
	localAddr net.IP
}

func discoverGateway(ctx context.Context) (*gateway, error) {
	if clients, _, err := internetgateway2.NewWANIPConnection2ClientsCtx(ctx); err == nil && len(clients) > 0 {
		return &gateway{clients[0], clients[0].Location, clients[0].LocalAddr()}, nil
	}
	if clients, _, err := internetgateway1.NewWANIPConnection1ClientsCtx(ctx); err == nil && len(clients) > 0 {
		return &gateway{clients[0], clients[0].Location, clients[0].LocalAddr()}, nil
	}
	return nil, errors.New("no UPnP IGD found within timeout")
}

func clientFromGateway(ctx context.Context, gatewayURL string) (*gateway, error) {
	location, err := url.Parse(gatewayURL)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway URL: %w", err)
	}
	if location.Scheme == "" || location.Host == "" {
		return nil, fmt.Errorf("invalid gateway URL %q: expected the IGD description URL, e.g. http://192.168.1.1:5000/rootDesc.xml", gatewayURL)
	}
	if clients, err := internetgateway2.NewWANIPConnection2ClientsByURLCtx(ctx, location); err == nil && len(clients) > 0 {
		return &gateway{clients[0], location, nil}, nil
	}
	if clients, err := internetgateway1.NewWANIPConnection1ClientsByURLCtx(ctx, location); err == nil && len(clients) > 0 {
		return &gateway{clients[0], location, nil}, nil
	}
	return nil, fmt.Errorf("no supported UPnP service found at %s", gatewayURL)
}

// Prefer the address that routes to the gateway, so that docker0, other bridges
// or VPN interfaces are not picked by mistake.
func getLocalIP(location *url.URL, discoveredFrom net.IP) (string, error) {
	if location != nil && location.Hostname() != "" {
		if ip, err := routeSourceIP(location.Hostname()); err == nil {
			return ip, nil
		}
	}

	if ip4 := discoveredFrom.To4(); ip4 != nil && !ip4.IsUnspecified() && !ip4.IsLoopback() {
		return ip4.String(), nil
	}

	return firstInterfaceIP()
}

func routeSourceIP(host string) (string, error) {
	// Dialing UDP sends no packets; it only selects the route and source address.
	conn, err := net.Dial("udp4", net.JoinHostPort(host, "1900"))
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()

	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP.IsUnspecified() || addr.IP.IsLoopback() {
		return "", errors.New("no usable source address")
	}

	return addr.IP.String(), nil
}

func firstInterfaceIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get interface addresses: %w", err)
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", errors.New("no valid local IP found")
}
