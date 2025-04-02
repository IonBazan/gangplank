package cmd

import (
	"log"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/spf13/cobra"
)

var (
	name   string
	addCmd = &cobra.Command{
		Use:   "add <external>:<internal>/<protocol>",
		Short: "Add a single UPnP port mapping",
		Long:  `Adds a single port mapping rule directly to the UPnP gateway for debugging purposes. Format: <external>:<internal>/<protocol> (e.g., 8080:80/tcp).`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			upnpClient, err := SetupUPnPClient()
			if err != nil {
				log.Fatalf("Failed to initialize UPnP client: %v", err)
			}

			mapping, err := types.ParsePortMapping(args[0])
			if err != nil {
				log.Fatalf("Failed to parse port mapping: %v", err)
			}
			mapping.Name = name

			if err := upnpClient.ForwardPorts([]types.PortMapping{mapping}); err != nil {
				log.Printf("Failed to add port mapping %d/%s: %v", mapping.ExternalPort, mapping.Protocol, err)
			} else {
				log.Printf("Successfully added port mapping %d/%s for %s", mapping.ExternalPort, mapping.Protocol, mapping.Name)
			}
		},
	}
)

func init() {
	addCmd.Flags().StringVar(&name, "name", "", "Optional name for the mapping")
}
