package cmd

import (
	"fmt"
	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"log"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
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
	SetupUPnPClient = func() (*upnp.Client, error) {
		if dryRun {
			return upnp.NewDummyClient(ttl), nil
		}

		return upnp.NewClient(localIP, gateway, ttl)
	}
	rootCmd = &cobra.Command{
		Use:     "gangplank",
		Short:   "Gangplank manages port mappings with UPnP",
		Long:    `Gangplank is a CLI tool to fetch port mappings from various sources and forward them via UPnP.`,
		Version: fmt.Sprintf("%s (commit: %s, created: %s)", version, commit, created),
	}
)

func Execute() {
	fmt.Print(banner)
	fmt.Printf("Running version %s built on %s (commit %s)\n", version, created, commit)

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
	rootCmd.PersistentFlags().DurationVar(&ttl, "ttl", upnp.DefaultLeaseDuration, "UPnP lease duration")

	rootCmd.AddCommand(forwardCmd)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(daemonCmd)
	rootCmd.AddCommand(listCmd)
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

	cfg, err := config.LoadConfig(configFile)
	if err != nil && configFile != "" {
		log.Fatalf("error loading config file: %v", err)
	}

	if cfg != nil {
		if cfg.RefreshInterval > 0 {
			viper.SetDefault("refresh-interval", cfg.RefreshInterval)
		}

		if cfg.LocalIP != "" {
			viper.SetDefault("local-ip", cfg.LocalIP)
		}

		if cfg.Ttl > 0 {
			viper.SetDefault("ttl", cfg.Ttl)
		}
	}

	bindFlags(rootCmd)

	for _, cmd := range rootCmd.Commands() {
		bindFlags(cmd)
	}
}
