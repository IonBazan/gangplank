package cmd

import (
	"github.com/IonBazan/gangplank/internal/upnp"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	dryRun          bool
	localIP         string
	gateway         string
	duration        time.Duration
	SetupUPnPClient = func() (*upnp.Client, error) {
		if dryRun {
			return upnp.NewDummyClient(duration)
		}

		localIPOverride := localIP
		if localIPOverride == "" {
			localIPOverride = os.Getenv("gangplank_LOCAL_IP")
		}
		gatewayOverride := gateway
		if gatewayOverride == "" {
			gatewayOverride = os.Getenv("gangplank_GATEWAY")
		}

		return upnp.NewClient(localIPOverride, gatewayOverride, duration)
	}
	rootCmd = &cobra.Command{
		Use:   "gangplank",
		Short: "Gangplank manages port mappings with UPnP",
		Long:  `Gangplank is a CLI tool to fetch port mappings from various sources and forward them via UPnP.`,
	}
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Do not apply changes - only list the ports")
	rootCmd.PersistentFlags().StringVar(&localIP, "local-ip", "", "Local IP address to use for UPnP (default: auto-detected)")
	rootCmd.PersistentFlags().StringVar(&gateway, "gateway", "", "UPnP gateway location URL (default: auto-detected)")
	rootCmd.PersistentFlags().DurationVar(&duration, "duration", upnp.DefaultLeaseDuration, "UPnP lease duration")
	rootCmd.AddCommand(forwardCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(deleteCmd)
}
