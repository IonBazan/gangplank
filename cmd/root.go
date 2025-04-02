package cmd

import (
	"fmt"
	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const envPrefix = "GANGPLANK"

var (
	configFile string
	cfg        *config.Config
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

		fmt.Println("Duration: ", duration)

		return upnp.NewClient(localIP, gateway, duration)
	}
	rootCmd = &cobra.Command{
		Use:   "gangplank",
		Short: "Gangplank manages port mappings with UPnP",
		Long:  `Gangplank is a CLI tool to fetch port mappings from various sources and forward them via UPnP.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Name() == "completion" || cmd.Name() == "help" {
				return nil
			}

			var err error
			cfg, err = config.LoadConfig(configFile)
			if err != nil && configFile != "" {
				return fmt.Errorf("error loading config file: %w", err)
			}

			if cfg == nil {
				cfg = &config.Config{}
			}

			if localIP == "" && cfg.LocalIP != "" {
				localIP = cfg.LocalIP
			}

			if gateway == "" && cfg.Gateway != "" {
				gateway = cfg.Gateway
			}

			if duration == upnp.DefaultLeaseDuration && cfg.Duration > 0 {
				duration = cfg.Duration
			}

			return nil
		},
	}
)

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Do not apply changes - only list the ports")
	rootCmd.PersistentFlags().StringVar(&localIP, "local-ip", "", "Local IP address to use for UPnP (default: auto-detected)")
	rootCmd.PersistentFlags().StringVar(&gateway, "gateway", "", "UPnP gateway location URL (default: auto-detected)")
	rootCmd.PersistentFlags().DurationVar(&duration, "duration", upnp.DefaultLeaseDuration, "UPnP lease duration")

	rootCmd.AddCommand(forwardCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(deleteCmd)
}

// bindFlags binds each cobra flag to its associated viper configuration
func bindFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		envVarName := strings.ToUpper(fmt.Sprintf("%s%s", envPrefix, strings.ReplaceAll(f.Name, "-", "_")))

		if !f.Changed && viper.IsSet(f.Name) {
			val := viper.Get(f.Name)
			cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val))
		}

		viper.BindEnv(f.Name, envVarName)
	})
}

func initConfig() {
	viper.AutomaticEnv()
	viper.SetEnvPrefix(envPrefix)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	bindFlags(rootCmd)

	for _, cmd := range rootCmd.Commands() {
		bindFlags(cmd)
	}
}
