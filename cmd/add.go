package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"

	"github.com/IonBazan/gangplank/internal/portmap"
)

func (a *app) addCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "add <external>:<internal>[/<protocol>]",
		Short: "Add a single UPnP port mapping",
		Long:  `Adds a single port mapping rule directly to the UPnP gateway. Format: <external>:<internal>[/<protocol>] (e.g., 8080:80/tcp). Protocol defaults to TCP.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mapping, err := portmap.Parse(args[0])
			if err != nil {
				return fmt.Errorf("failed to parse port mapping: %w", err)
			}
			mapping.Name = name

			gateway, err := a.connectGateway(cmd.Context())
			if err != nil {
				return err
			}

			if err := gateway.ForwardPorts(cmd.Context(), []portmap.Mapping{mapping}); err != nil {
				return err
			}
			log.Printf("Successfully added port mapping %s", mapping.Key())
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Optional name for the mapping")

	return cmd
}
