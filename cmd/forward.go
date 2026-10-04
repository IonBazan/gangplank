package cmd

import (
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/IonBazan/gangplank/internal/gangplank"
)

func (a *app) forwardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "forward",
		Short: "Fetch and forward port mappings",
		Long:  `Fetches port mappings from Docker and YAML sources and forwards them via UPnP with Gangplank.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			dockerCli, err := a.newDocker()
			if err != nil {
				return err
			}
			defer func() { _ = dockerCli.Close() }()

			gateway, err := a.connectGateway(ctx)
			if err != nil {
				return err
			}
			slog.Info("Connected to UPnP gateway", "local_ip", gateway.InternalIP())

			manager := gangplank.NewManager(a.cfg, dockerCli)
			manager.SetGateway(gateway)
			ports, err := manager.Refresh(ctx, false)
			for _, p := range ports {
				gangplank.LogMapping(p)
			}
			if err != nil {
				return fmt.Errorf("some port mappings could not be forwarded: %w", err)
			}

			return nil
		},
	}
}
