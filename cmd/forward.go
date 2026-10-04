package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	"github.com/IonBazan/gangplank/internal"
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/spf13/cobra"
)

var (
	forwardCmd = &cobra.Command{
		Use:   "forward",
		Short: "Fetch and forward port mappings",
		Long:  `Fetches port mappings from Docker and YAML sources and forwards them via UPnP with Gangplank.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			log.Println("Starting Gangplank...")
			dockerCli, err := NewDockerClient()
			if err != nil {
				return err
			}
			defer dockerCli.Close()

			gp := internal.NewGangplank(cfg, dockerCli)
			ports, fetchErr := gp.GetPortMappings(ctx)
			listPorts(ports)

			upnpClient, err := SetupUPnPClient(ctx)
			if err != nil {
				return errors.Join(fetchErr, err)
			}
			log.Printf("UPnP client initialized with local IP: %s", upnpClient.LocalIP)
			gp.SetForwarder(upnpClient)

			if err := gp.ForwardPorts(ctx, ports); err != nil {
				return errors.Join(fetchErr, fmt.Errorf("some port mappings could not be applied: %w", err))
			}

			return fetchErr
		},
	}
)

func listPorts(ports []types.PortMapping) {
	for _, p := range ports {
		if p.Name != "" {
			log.Printf("Port Mapping (Container: %s): External=%d, Internal=%d, Protocol=%s\n", p.Name, p.ExternalPort, p.InternalPort, p.Protocol)
		} else {
			log.Printf("Port Mapping: External=%d, Internal=%d, Protocol=%s\n", p.ExternalPort, p.InternalPort, p.Protocol)
		}
	}
}
