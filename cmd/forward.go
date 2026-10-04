package cmd

import (
	"errors"
	"fmt"
	"log"
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

			log.Println("Starting Gangplank...")
			dockerCli, err := a.newDocker()
			if err != nil {
				return err
			}
			defer func() { _ = dockerCli.Close() }()

			manager := gangplank.NewManager(a.cfg, dockerCli)
			ports, fetchErr := manager.GetPortMappings(ctx)
			gangplank.LogMappings(ports)

			gateway, err := a.connectGateway(ctx)
			if err != nil {
				return errors.Join(fetchErr, err)
			}
			log.Printf("UPnP client initialized with local IP: %s", gateway.InternalIP())
			manager.SetGateway(gateway)

			if err := manager.ForwardPorts(ctx, ports); err != nil {
				return errors.Join(fetchErr, fmt.Errorf("some port mappings could not be applied: %w", err))
			}

			return fetchErr
		},
	}
}
