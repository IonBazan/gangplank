package cmd

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/IonBazan/gangplank/internal"
)

const (
	upnpRetryInterval = 30 * time.Second
	shutdownTimeout   = 10 * time.Second
)

var (
	cleanupOnStop   bool
	cleanupOnExit   bool
	prune           bool
	poll            bool
	refreshInterval time.Duration
	daemonCmd       = &cobra.Command{
		Use:   "daemon",
		Short: "Run as a daemon with polling and port refreshing",
		Long:  `Runs Gangplank as a daemon, listening for container events and refreshing port mappings at intervals.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if refreshInterval <= 0 {
				return errors.New("--refresh-interval must be greater than 0")
			}
			if ttl > 0 && refreshInterval >= ttl {
				log.Printf("Warning: refresh interval (%s) is not shorter than the lease TTL (%s); mappings may expire between refreshes", refreshInterval, ttl)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			log.Println("Starting Gangplank daemon...")
			dockerCli, err := NewDockerClient()
			if err != nil {
				return err
			}
			defer func() { _ = dockerCli.Close() }()
			log.Printf("Using Docker daemon at %s", dockerCli.DaemonHost())

			gp := internal.NewGangplank(cfg, dockerCli)

			var wg sync.WaitGroup
			if poll {
				wg.Add(1)
				go func() {
					defer wg.Done()
					gp.PollAndForward(ctx, cleanupOnStop)
				}()
			}

			refresh := func() {
				if !connectUPnP(ctx, gp) {
					return
				}
				ports, err := gp.GetPortMappings(ctx)
				if err != nil {
					log.Printf("Some port mappings could not be fetched: %v", err)
				}
				listPorts(ports)
				if err := gp.Sync(ctx, ports, prune); err != nil {
					log.Printf("Some port mappings could not be applied: %v", err)
				}
			}

			refresh()
			for {
				wait := refreshInterval
				if !gp.HasForwarder() {
					wait = min(upnpRetryInterval, refreshInterval)
				}

				select {
				case <-ctx.Done():
					log.Println("Shutting down...")
					wg.Wait()
					if cleanupOnExit {
						cleanupCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
						defer cancel()
						if err := gp.Cleanup(cleanupCtx); err != nil {
							log.Printf("Failed to clean up port mappings: %v", err)
						}
					}
					return nil
				case <-time.After(wait):
					log.Printf("Refreshing port mappings...")
					refresh()
				}
			}
		},
	}
)

// Retried on every refresh, so a router that is still booting is picked up later.
func connectUPnP(ctx context.Context, gp *internal.Gangplank) bool {
	if gp.HasForwarder() {
		return true
	}

	upnpClient, err := SetupUPnPClient(ctx)
	if err != nil {
		log.Printf("Failed to initialize UPnP client: %v, retrying later", err)
		return false
	}

	log.Printf("UPnP client initialized with local IP: %s", upnpClient.LocalIP)
	gp.SetForwarder(upnpClient)
	return true
}

func init() {
	daemonCmd.Flags().BoolVarP(&poll, "poll", "p", false, "Listen for container events")
	daemonCmd.Flags().BoolVar(&cleanupOnStop, "cleanup-on-stop", false, "Delete port mappings on container stop/die")
	daemonCmd.Flags().BoolVar(&cleanupOnExit, "cleanup-on-exit", false, "Delete forwarded port mappings when the daemon stops")
	daemonCmd.Flags().BoolVar(&prune, "prune", false, "On refresh, delete Gangplank mappings for this host that are no longer wanted")
	daemonCmd.Flags().DurationVar(&refreshInterval, "refresh-interval", 15*time.Minute, "Interval to refresh port mappings")
}
