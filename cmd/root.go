package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/docker/docker/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const banner = `
░█▀▀░█▀█░█▀█░█▀▀░█▀█░█░░░█▀█░█▀█░█░█
░█░█░█▀█░█░█░█░█░█▀▀░█░░░█▀█░█░█░█▀▄
░▀▀▀░▀░▀░▀░▀░▀▀▀░▀░░░▀▀▀░▀░▀░▀░▀░▀░▀
`

const envPrefix = "GANGPLANK"

//nolint:gochecknoglobals
var (
	version = "unknown"
	commit  = "unknown"
	created = "an unknown date"
)

var (
	configFile string
	cfg        *config.Config
)

var (
	dryRun          bool
	localIP         string
	gateway         string
	ttl             time.Duration
	SetupUPnPClient = func(ctx context.Context) (*upnp.Client, error) {
		if dryRun {
			return upnp.NewDummyClient(ttl), nil
		}

		return upnp.NewClient(ctx, localIP, gateway, ttl)
	}
	NewDockerClient = func() (*client.Client, error) {
		return client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	}
	rootCmd = &cobra.Command{
		Use:          "gangplank",
		Short:        "Gangplank manages port mappings with UPnP",
		Long:         `Gangplank is a CLI tool to fetch port mappings from various sources and forward them via UPnP.`,
		Version:      fmt.Sprintf("%s (commit: %s, created: %s)", version, commit, created),
		SilenceUsage: true,
	}
)

func Execute() {
	// Diagnostics go to stderr so command output (e.g. list) stays pipeable.
	fmt.Fprint(os.Stderr, banner)
	fmt.Fprintf(os.Stderr, "Running version %s built on %s (commit %s)\n", version, created, commit)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path (default: ./config.yaml if present)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Do not apply changes - only list the ports")
	rootCmd.PersistentFlags().StringVar(&localIP, "local-ip", "", "Local IP address to use for UPnP (default: auto-detected)")
	rootCmd.PersistentFlags().StringVar(&gateway, "gateway", "", "UPnP gateway description URL, e.g. http://192.168.1.1:5000/rootDesc.xml (default: auto-detected)")
	rootCmd.PersistentFlags().DurationVar(&ttl, "ttl", upnp.DefaultLeaseDuration, "UPnP lease duration")

	rootCmd.AddCommand(forwardCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(listCmd)
}

// envVarName returns the environment variable for a flag, e.g. refresh-interval -> GANGPLANK_REFRESH_INTERVAL.
func envVarName(flag string) string {
	return envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(flag, "-", "_"))
}

// bindFlags fills flags not set on the command line from the environment or config file.
// Precedence: flag > environment variable > config file > default.
func bindFlags(flags *pflag.FlagSet) {
	flags.VisitAll(func(f *pflag.Flag) {
		_ = viper.BindEnv(f.Name, envVarName(f.Name))

		if !f.Changed && viper.IsSet(f.Name) {
			if err := flags.Set(f.Name, viper.GetString(f.Name)); err != nil {
				log.Fatalf("invalid value for %s: %v", f.Name, err)
			}
		}
	})
}

func initConfig() {
	var err error
	cfg, err = config.LoadConfig(configFile)
	if err != nil {
		var notFound viper.ConfigFileNotFoundError
		if configFile != "" || !errors.As(err, &notFound) {
			log.Fatalf("error loading config file: %v", err)
		}
		cfg = nil
	}

	if cfg != nil {
		if cfg.RefreshInterval > 0 {
			viper.SetDefault("refresh-interval", cfg.RefreshInterval)
		}

		if cfg.Gateway != "" {
			viper.SetDefault("gateway", cfg.Gateway)
		}

		if cfg.LocalIP != "" {
			viper.SetDefault("local-ip", cfg.LocalIP)
		}

		if cfg.Ttl > 0 {
			viper.SetDefault("ttl", cfg.Ttl)
		}
	}

	bindFlags(rootCmd.PersistentFlags())

	for _, cmd := range rootCmd.Commands() {
		bindFlags(cmd.Flags())
	}
}
