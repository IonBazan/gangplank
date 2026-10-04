package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/IonBazan/gangplank/internal/portmap"
)

func (a *app) deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <external>[/<protocol>]",
		Short: "Delete a single UPnP port mapping",
		Long:  `Deletes a single port mapping rule directly from the UPnP gateway. Format: <external>[/<protocol>] (e.g., 8080/tcp). Protocol defaults to TCP.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mapping, err := portmap.Parse(args[0])
			if err != nil {
				return fmt.Errorf("failed to parse port mapping: %w", err)
			}

			gateway, err := a.connectGateway(cmd.Context())
			if err != nil {
				return err
			}

			if err := gateway.DeletePortMapping(cmd.Context(), mapping.ExternalPort, mapping.Protocol); err != nil {
				return fmt.Errorf("failed to delete port mapping %s: %w", mapping.Key(), err)
			}
			log.Printf("Successfully deleted port mapping %s", mapping.Key())
			return nil
		},
	}
}
