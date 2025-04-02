package cmd

import (
	"context"
	"github.com/IonBazan/gangplank/internal"
	"log"
	"time"

	"github.com/spf13/cobra"
)

var (
	cleanupOnStop   bool
	poll            bool
	refreshInterval time.Duration
	daemonCmd       = &cobra.Command{
		Use:   "daemon",
		Short: "Run as a daemon with polling and port refreshing",
		Long:  `Runs Gangplank as a daemon, listening for container events and refreshing port mappings at intervals.`,
		Run: func(cmd *cobra.Command, args []string) {
			upnpClient, err := SetupUPnPClient()
			if err != nil {
				log.Printf("Failed to initialize UPnP client: %v, proceeding without UPnP forwarding", err)
			} else {
				log.Printf("UPnP client initialized with local IP: %s", upnpClient.LocalIP)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			gp := internal.NewGangplank(cfg, upnpClient)

			initialPorts, _ := gp.FetchPorts()

			listPorts(initialPorts)
			gp.ForwardPorts(initialPorts)

			go func() {
				ticker := time.NewTicker(refreshInterval)
				defer ticker.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						log.Printf("Updating port mappings...")
						ports, _ := gp.FetchPorts()
						if len(ports) > 0 {
							log.Printf("Refreshing %d port mappings", len(ports))
							gp.ForwardPorts(ports)
						}
					}
				}
			}()

			if poll {
				go gp.PollAndForward(ctx, cleanupOnStop)
			}

			select {}
		},
	}
)

func init() {
	daemonCmd.Flags().BoolVarP(&poll, "poll", "p", false, "Listen for container events")
	daemonCmd.Flags().BoolVar(&cleanupOnStop, "cleanup-on-stop", false, "Delete port mappings on container stop/die")
	daemonCmd.Flags().DurationVar(&refreshInterval, "refresh-interval", 15*time.Minute, "Interval to refresh port mappings")
}
