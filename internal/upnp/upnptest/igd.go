// Package upnptest provides a fake UPnP Internet Gateway Device for tests.
package upnptest

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const serviceType = "urn:schemas-upnp-org:service:WANIPConnection:1"

const rootDesc = `<?xml version="1.0"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <device>
    <deviceType>urn:schemas-upnp-org:device:InternetGatewayDevice:1</deviceType>
    <friendlyName>Fake IGD</friendlyName>
    <UDN>uuid:fake-igd</UDN>
    <deviceList>
      <device>
        <deviceType>urn:schemas-upnp-org:device:WANDevice:1</deviceType>
        <UDN>uuid:fake-wan</UDN>
        <deviceList>
          <device>
            <deviceType>urn:schemas-upnp-org:device:WANConnectionDevice:1</deviceType>
            <UDN>uuid:fake-wan-conn</UDN>
            <serviceList>
              <service>
                <serviceType>` + serviceType + `</serviceType>
                <serviceId>urn:upnp-org:serviceId:WANIPConn1</serviceId>
                <controlURL>/ctl</controlURL>
                <eventSubURL>/evt</eventSubURL>
                <SCPDURL>/scpd.xml</SCPDURL>
              </service>
            </serviceList>
          </device>
        </deviceList>
      </device>
    </deviceList>
  </device>
</root>`

type Mapping struct {
	ExternalPort int
	InternalPort int
	Protocol     string
	InternalIP   string
	Description  string
	Lease        int
}

type IGD struct {
	Server *httptest.Server
	// PermanentOnly makes the gateway reject non-zero leases with error 725.
	PermanentOnly bool
	ExternalIP    string

	mu       sync.Mutex
	mappings map[string]Mapping
}

func NewIGD(t interface{ Cleanup(func()) }) *IGD {
	g := &IGD{ExternalIP: "203.0.113.7", mappings: map[string]Mapping{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/rootDesc.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		_, _ = io.WriteString(w, rootDesc)
	})
	mux.HandleFunc("/ctl", g.control)
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Server.Close)

	return g
}

func (g *IGD) URL() string {
	return g.Server.URL + "/rootDesc.xml"
}

func (g *IGD) Mappings() []Mapping {
	g.mu.Lock()
	defer g.mu.Unlock()

	result := make([]Mapping, 0, len(g.mappings))
	for _, m := range g.mappings {
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Protocol != result[j].Protocol {
			return result[i].Protocol < result[j].Protocol
		}
		return result[i].ExternalPort < result[j].ExternalPort
	})

	return result
}

func (g *IGD) Add(m Mapping) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mappings[key(m.ExternalPort, m.Protocol)] = m
}

func key(port int, protocol string) string {
	return fmt.Sprintf("%d/%s", port, protocol)
}

func (g *IGD) control(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(r.Header.Get("SOAPACTION"), `"`)
	_, name, _ := strings.Cut(action, "#")

	args, err := parseArgs(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	switch name {
	case "GetExternalIPAddress":
		respond(w, name, map[string]string{"NewExternalIPAddress": g.ExternalIP})
	case "AddPortMapping":
		lease, _ := strconv.Atoi(args["NewLeaseDuration"])
		if g.PermanentOnly && lease != 0 {
			fault(w, 725, "OnlyPermanentLeasesSupported")
			return
		}
		ext, _ := strconv.Atoi(args["NewExternalPort"])
		internal, _ := strconv.Atoi(args["NewInternalPort"])
		g.mappings[key(ext, args["NewProtocol"])] = Mapping{
			ExternalPort: ext,
			InternalPort: internal,
			Protocol:     args["NewProtocol"],
			InternalIP:   args["NewInternalClient"],
			Description:  args["NewPortMappingDescription"],
			Lease:        lease,
		}
		respond(w, name, nil)
	case "DeletePortMapping":
		ext, _ := strconv.Atoi(args["NewExternalPort"])
		k := key(ext, args["NewProtocol"])
		if _, ok := g.mappings[k]; !ok {
			fault(w, 714, "NoSuchEntryInArray")
			return
		}
		delete(g.mappings, k)
		respond(w, name, nil)
	case "GetGenericPortMappingEntry":
		index, _ := strconv.Atoi(args["NewPortMappingIndex"])
		keys := make([]string, 0, len(g.mappings))
		for k := range g.mappings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if index >= len(keys) {
			fault(w, 713, "SpecifiedArrayIndexInvalid")
			return
		}
		m := g.mappings[keys[index]]
		respond(w, name, map[string]string{
			"NewRemoteHost":             "",
			"NewExternalPort":           strconv.Itoa(m.ExternalPort),
			"NewProtocol":               m.Protocol,
			"NewInternalPort":           strconv.Itoa(m.InternalPort),
			"NewInternalClient":         m.InternalIP,
			"NewEnabled":                "1",
			"NewPortMappingDescription": m.Description,
			"NewLeaseDuration":          strconv.Itoa(m.Lease),
		})
	default:
		fault(w, 401, "Invalid Action")
	}
}

func parseArgs(body io.Reader) (map[string]string, error) {
	args := map[string]string{}
	decoder := xml.NewDecoder(body)
	var current string
	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return args, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			current = t.Name.Local
		case xml.CharData:
			if current != "" {
				args[current] += string(t)
			}
		case xml.EndElement:
			current = ""
		}
	}
}

func respond(w http.ResponseWriter, action string, values map[string]string) {
	var b strings.Builder
	for k, v := range values {
		b.WriteString("<" + k + ">")
		_ = xml.EscapeText(&b, []byte(v))
		b.WriteString("</" + k + ">")
	}
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body><u:%sResponse xmlns:u="%s">%s</u:%sResponse></s:Body>
</s:Envelope>`, action, serviceType, b.String(), action)
}

func fault(w http.ResponseWriter, code int, description string) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
<s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring>
<detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0"><errorCode>%d</errorCode><errorDescription>%s</errorDescription></UPnPError></detail>
</s:Fault></s:Body>
</s:Envelope>`, code, description)
}
