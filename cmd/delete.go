package cmd

import (
	"log"

	"github.com/IonBazan/gangplank/internal/types"
	"github.com/spf13/cobra"
)

var (
	deleteCmd = &cobra.Command{
		Use:   "delete <external>:<internal>/<protocol>",
		Short: "Delete a single UPnP port mapping",
		Long:  `Deletes a single port mapping rule directly from the UPnP gateway for debugging purposes. Format: <external>:<internal>/<protocol> (e.g., 8080:80/tcp). Note: internal port is ignored for deletion.`,
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

			if err := upnpClient.DeletePortMapping(mapping.ExternalPort, mapping.Protocol); err != nil {
				log.Printf("Failed to delete port mapping %d/%s: %v", mapping.ExternalPort, mapping.Protocol, err)
			} else {
				log.Printf("Successfully deleted port mapping %d/%s", mapping.ExternalPort, mapping.Protocol)
			}
		},
	}
)

func init() {
}
