package cmd

import (
	"context"
	"errors"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/IonBazan/gangplank/internal/gangplank"
)

func (a *app) daemonCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as a daemon with polling and port refreshing",
		Long:  `Runs Gangplank as a daemon, listening for container events and refreshing port mappings at intervals.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.opts.refreshInterval <= 0 {
				return errors.New("--refresh-interval must be greater than 0")
			}
			if a.opts.ttl > 0 && a.opts.refreshInterval >= a.opts.ttl {
				log.Printf("Warning: refresh interval (%s) is not shorter than the lease TTL (%s); mappings may expire between refreshes", a.opts.refreshInterval, a.opts.ttl)
			}

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			log.Println("Starting Gangplank daemon...")
			dockerCli, err := a.newDocker()
			if err != nil {
				return err
			}
			defer func() { _ = dockerCli.Close() }()
			log.Printf("Using Docker daemon at %s", dockerCli.DaemonHost())

			connect := func(ctx context.Context) (gangplank.Gateway, error) {
				gateway, err := a.connectGateway(ctx)
				if err != nil {
					return nil, err
				}
				return gateway, nil
			}

			gangplank.NewDaemon(gangplank.NewManager(a.cfg, dockerCli), connect, gangplank.DaemonOptions{
				RefreshInterval: a.opts.refreshInterval,
				Poll:            a.opts.poll,
				CleanupOnStop:   a.opts.cleanupOnStop,
				CleanupOnExit:   a.opts.cleanupOnExit,
				Prune:           a.opts.prune,
			}).Run(ctx)

			return nil
		},
	}

	flags := cmd.Flags()
	flags.BoolVarP(&a.opts.poll, "poll", "p", false, "Listen for container events")
	flags.BoolVar(&a.opts.cleanupOnStop, "cleanup-on-stop", false, "Delete port mappings on container stop/die")
	flags.BoolVar(&a.opts.cleanupOnExit, "cleanup-on-exit", false, "Delete forwarded port mappings when the daemon stops")
	flags.BoolVar(&a.opts.prune, "prune", false, "On refresh, delete Gangplank mappings for this host that are no longer wanted")
	flags.DurationVar(&a.opts.refreshInterval, "refresh-interval", 15*time.Minute, "Interval to refresh port mappings")

	return cmd
}
