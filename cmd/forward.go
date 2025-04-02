package cmd

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/IonBazan/gangplank/internal/fetchers"
	"github.com/IonBazan/gangplank/internal/types"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

var (
	cleanupOnStop   bool
	poll            bool
	daemon          bool
	refreshInterval time.Duration
	forwardCmd      = &cobra.Command{
		Use:   "forward",
		Short: "Fetch and forward port mappings",
		Long:  `Fetches port mappings from Docker and YAML sources and forwards them via UPnP with Gangplank.`,
		Run: func(cmd *cobra.Command, args []string) {
			dockerCli, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
			if err != nil {
				log.Fatalf("Failed to create Docker client: %v", err)
			}

			upnpClient, err := SetupUPnPClient()
			if err != nil {
				log.Printf("Failed to initialize UPnP client: %v, proceeding without UPnP forwarding", err)
			} else {
				log.Printf("UPnP client initialized with local IP: %s", upnpClient.LocalIP)
			}

			portFetchers := []types.PortFetcher{
				fetchers.NewConfigPortFetcher(cfg),
				fetchers.NewDockerPortFetcher(dockerCli),
			}
			eventFetchers := []types.EventPortFetcher{
				fetchers.NewDockerEventPortFetcher(dockerCli),
			}

			var initialPorts []types.PortMapping
			for _, fetcher := range portFetchers {
				ports, err := fetcher.FetchPorts()
				if err != nil {
					log.Printf("Error fetching ports: %v", err)
					continue
				}
				initialPorts = append(initialPorts, ports...)
			}

			listPorts(initialPorts)
			if upnpClient != nil {
				forwardPorts(upnpClient, initialPorts)
			}

			if daemon {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				if poll {
					go pollAndForward(ctx, eventFetchers, upnpClient, cleanupOnStop)
				}

				if upnpClient != nil {
					go refreshPorts(ctx, upnpClient, portFetchers, refreshInterval)
				}

				select {}
			}
		},
	}
)

func init() {
	forwardCmd.Flags().BoolVar(&cleanupOnStop, "cleanup-on-stop", false, "Delete port mappings on container stop/die")
	forwardCmd.Flags().BoolVarP(&poll, "poll", "p", false, "Listen for container events (requires --daemon)")
	forwardCmd.Flags().BoolVarP(&daemon, "daemon", "d", false, "Run as a daemon with polling and port refreshing")
	forwardCmd.Flags().DurationVar(&refreshInterval, "refresh-interval", 15*time.Minute, "Interval to refresh port mappings (used with --daemon)")
}

func listPorts(ports []types.PortMapping) {
	for _, p := range ports {
		if p.Name != "" {
			fmt.Printf("Port Mapping (Container: %s): External=%d, Internal=%d, Protocol=%s\n", p.Name, p.ExternalPort, p.InternalPort, p.Protocol)
		} else {
			fmt.Printf("Port Mapping: External=%d, Internal=%d, Protocol=%s\n", p.ExternalPort, p.InternalPort, p.Protocol)
		}
	}
}

func forwardPorts(upnpClient *upnp.Client, ports []types.PortMapping) {
	if err := upnpClient.ForwardPorts(ports); err != nil {
		log.Printf("Error forwarding ports: %v", err)
	}
}

func pollAndForward(ctx context.Context, eventFetchers []types.EventPortFetcher, upnpClient *upnp.Client, cleanup bool) {
	addCh := make(chan types.PortMapping)
	deleteCh := make(chan types.PortMapping)

	for _, fetcher := range eventFetchers {
		go fetcher.Listen(ctx, addCh, deleteCh)
	}

	if upnpClient != nil && cleanup {
		go func() {
			for m := range deleteCh {
				if err := upnpClient.DeletePortMapping(m.ExternalPort, m.Protocol); err != nil {
					log.Printf("Failed to delete port mapping %d/%s for %s: %v", m.ExternalPort, m.Protocol, m.Name, err)
				} else {
					log.Printf("Deleted port mapping %d/%s for %s", m.ExternalPort, m.Protocol, m.Name)
				}
			}
		}()
	}

	for p := range addCh {
		fmt.Printf("New Container Port Mapping (Container: %s): External=%d, Internal=%d, Protocol=%s\n", p.Name, p.ExternalPort, p.InternalPort, p.Protocol)
		if upnpClient != nil {
			if err := upnpClient.ForwardPorts([]types.PortMapping{p}); err != nil {
				log.Printf("Error forwarding new port: %v", err)
			}
		}
	}
}

func refreshPorts(ctx context.Context, upnpClient *upnp.Client, fetchers []types.PortFetcher, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("Updating port mappings...")
			var ports []types.PortMapping
			for _, fetcher := range fetchers {
				fetchedPorts, err := fetcher.FetchPorts()
				if err != nil {
					log.Printf("Error fetching ports for refresh: %v", err)
					continue
				}
				ports = append(ports, fetchedPorts...)
			}
			if len(ports) > 0 {
				log.Printf("Refreshing %d port mappings", len(ports))
				forwardPorts(upnpClient, ports)
			}
		}
	}
}
