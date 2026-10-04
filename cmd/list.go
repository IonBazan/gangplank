package cmd

import (
	"fmt"
	"log/slog"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func (a *app) listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all active UPnP port mappings",
		Long:  `Retrieves and displays all active UPnP port mappings from the gateway, including external port, internal port, protocol, internal IP, description, and lease duration.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			gateway, err := a.connectGateway(cmd.Context())
			if err != nil {
				return err
			}
			mappings, err := gateway.ListPortMappings(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list port mappings: %w", err)
			}

			if len(mappings) == 0 {
				slog.Info("No active UPnP port mappings found")
				return nil
			}

			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "External Port\tInternal Port\tProtocol\tInternal IP\tDescription\tLease Duration\tEnabled")
			_, _ = fmt.Fprintln(w, "-------------\t-------------\t--------\t-----------\t-----------\t--------------\t--------")
			for _, mapping := range mappings {
				leaseDuration := "Permanent"
				if mapping.LeaseDuration > 0 {
					leaseDuration = fmt.Sprintf("%d seconds", mapping.LeaseDuration)
				}
				_, _ = fmt.Fprintf(w, "%d\t%d\t%s\t%s\t%s\t%s\t%t\n",
					mapping.ExternalPort,
					mapping.InternalPort,
					mapping.Protocol,
					mapping.InternalIP,
					mapping.Description,
					leaseDuration,
					mapping.Enabled,
				)
			}
			return w.Flush()
		},
	}
}
