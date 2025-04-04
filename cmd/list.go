package cmd

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active UPnP port mappings",
	Long:  `Retrieves and displays all active UPnP port mappings from the gateway, including external port, internal port, protocol, internal IP, description, and lease duration.`,
	Args:  cobra.NoArgs, // No arguments required
	Run: func(cmd *cobra.Command, args []string) {
		upnpClient, err := SetupUPnPClient()
		if err != nil {
			log.Fatalf("Failed to initialize UPnP client: %v", err)
		}
		mappings, err := upnpClient.ListPortMappings()
		if err != nil {
			log.Fatalf("Failed to list port mappings: %v", err)
		}

		if len(mappings) == 0 {
			log.Println("No active UPnP port mappings found.")
			return
		}

		fmt.Println("Active UPnP Port Mappings:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "External Port\tInternal Port\tProtocol\tInternal IP\tDescription\tLease Duration\tEnabled")
		fmt.Fprintln(w, "-------------\t-------------\t--------\t-----------\t-----------\t--------------\t--------")
		for _, mapping := range mappings {
			leaseDuration := "Permanent"
			if mapping.LeaseDuration > 0 {
				leaseDuration = fmt.Sprintf("%d seconds", mapping.LeaseDuration)
			}
			fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\t%s\t%t\n",
				mapping.ExternalPort,
				mapping.InternalPort,
				mapping.Protocol,
				mapping.InternalIP,
				mapping.Description,
				leaseDuration,
				mapping.Enabled,
			)
		}
		w.Flush()
	},
}

func init() {
}
