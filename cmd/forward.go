package cmd

import (
	"github.com/IonBazan/gangplank/internal"
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/spf13/cobra"
	"log"
)

var (
	forwardCmd = &cobra.Command{
		Use:   "forward",
		Short: "Fetch and forward port mappings",
		Long:  `Fetches port mappings from Docker and YAML sources and forwards them via UPnP with Gangplank.`,
		Run: func(cmd *cobra.Command, args []string) {
			upnpClient, err := SetupUPnPClient()
			if err != nil {
				log.Printf("Failed to initialize UPnP client: %v, proceeding without UPnP forwarding", err)
			} else {
				log.Printf("UPnP client initialized with local IP: %s", upnpClient.LocalIP)
			}

			gp := internal.NewGangplank(cfg, upnpClient)

			initialPorts, _ := gp.FetchPorts()

			listPorts(initialPorts)
			gp.ForwardPorts(initialPorts)
		},
	}
)

func init() {
}

func listPorts(ports []types.PortMapping) {
	for _, p := range ports {
		if p.Name != "" {
			log.Printf("Port Mapping (Container: %s): External=%d, Internal=%d, Protocol=%s\n", p.Name, p.ExternalPort, p.InternalPort, p.Protocol)
		} else {
			log.Printf("Port Mapping: External=%d, Internal=%d, Protocol=%s\n", p.ExternalPort, p.InternalPort, p.Protocol)
		}
	}
}
