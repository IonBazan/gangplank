package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/IonBazan/gangplank/internal/types"
)

var (
	name   string
	addCmd = &cobra.Command{
		Use:   "add <external>:<internal>[/<protocol>]",
		Short: "Add a single UPnP port mapping",
		Long:  `Adds a single port mapping rule directly to the UPnP gateway. Format: <external>:<internal>[/<protocol>] (e.g., 8080:80/tcp). Protocol defaults to TCP.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mapping, err := types.ParsePortMapping(args[0])
			if err != nil {
				return fmt.Errorf("failed to parse port mapping: %w", err)
			}
			mapping.Name = name

			upnpClient, err := SetupUPnPClient(cmd.Context())
			if err != nil {
				return err
			}

			if err := upnpClient.ForwardPorts(cmd.Context(), []types.PortMapping{mapping}); err != nil {
				return err
			}
			log.Printf("Successfully added port mapping %s", mapping.Key())
			return nil
		},
	}
)

func init() {
	addCmd.Flags().StringVar(&name, "name", "", "Optional name for the mapping")
}
