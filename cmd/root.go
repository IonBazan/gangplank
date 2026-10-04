package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/upnp"
)

const banner = `
░█▀▀░█▀█░█▀█░█▀▀░█▀█░█░░░█▀█░█▀█░█░█
░█░█░█▀█░█░█░█░█░█▀▀░█░░░█▀█░█░█░█▀▄
░▀▀▀░▀░▀░▀░▀░▀▀▀░▀░░░▀▀▀░▀░▀░▀░▀░▀░▀
`

const envPrefix = "GANGPLANK"

// Set at build time with -ldflags.
var (
	version = "unknown"
	commit  = "unknown"
	created = "an unknown date"
)

type options struct {
	configFile string
	dryRun     bool
	localIP    string
	gateway    string
	ttl        time.Duration

	poll            bool
	cleanupOnStop   bool
	cleanupOnExit   bool
	prune           bool
	refreshInterval time.Duration
}

type app struct {
	opts options
	cfg  *config.Config

	connectGateway func(ctx context.Context) (*upnp.Client, error)
	newDocker      func() (*client.Client, error)
}

func newApp() *app {
	a := &app{}
	a.connectGateway = a.defaultGateway
	a.newDocker = func() (*client.Client, error) {
		return client.New(client.FromEnv)
	}

	return a
}

func (a *app) defaultGateway(ctx context.Context) (*upnp.Client, error) {
	if a.opts.dryRun {
		return upnp.NewDryRunClient(a.opts.ttl), nil
	}

	return upnp.NewClient(ctx, a.opts.localIP, a.opts.gateway, a.opts.ttl)
}

func Execute() {
	// Keep stdout clean so command output can be piped.
	fmt.Fprint(os.Stderr, banner)
	fmt.Fprintf(os.Stderr, "Running version %s built on %s (commit %s)\n", version, created, commit)

	if err := newApp().rootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func (a *app) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:          "gangplank",
		Short:        "Gangplank manages port mappings with UPnP",
		Long:         `Gangplank is a CLI tool to fetch port mappings from various sources and forward them via UPnP.`,
		Version:      fmt.Sprintf("%s (commit: %s, created: %s)", version, commit, created),
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return a.loadSettings(cmd.Flags())
		},
	}

	flags := root.PersistentFlags()
	flags.StringVarP(&a.opts.configFile, "config", "c", "", "config file path (default: ./config.yaml if present)")
	flags.BoolVar(&a.opts.dryRun, "dry-run", false, "Do not apply changes - only list the ports")
	flags.StringVar(&a.opts.localIP, "local-ip", "", "Local IP address to use for UPnP (default: auto-detected)")
	flags.StringVar(&a.opts.gateway, "gateway", "", "UPnP gateway description URL, e.g. http://192.168.1.1:5000/rootDesc.xml (default: auto-detected)")
	flags.DurationVar(&a.opts.ttl, "ttl", upnp.DefaultLeaseDuration, "UPnP lease duration")

	root.AddCommand(a.forwardCmd(), a.addCmd(), a.deleteCmd(), a.daemonCmd(), a.listCmd())

	return root
}

func envVarName(flag string) string {
	return envPrefix + "_" + strings.ToUpper(strings.ReplaceAll(flag, "-", "_"))
}

// loadSettings fills the flags that were not given on the command line.
// Precedence: flag > environment variable > config file > default.
func (a *app) loadSettings(flags *pflag.FlagSet) error {
	if err := setFromEnv(flags, flags.Lookup("config")); err != nil {
		return err
	}

	cfg, err := config.Load(a.opts.configFile)
	if err != nil {
		return fmt.Errorf("error loading config file: %w", err)
	}
	a.cfg = cfg
	fromConfig := cfg.FlagValues()

	var firstErr error
	flags.VisitAll(func(f *pflag.Flag) {
		if firstErr != nil || f.Changed {
			return
		}
		if err := setFromEnv(flags, f); err != nil || f.Changed {
			firstErr = err
			return
		}
		if value, ok := fromConfig[f.Name]; ok {
			if err := flags.Set(f.Name, value); err != nil {
				firstErr = fmt.Errorf("invalid %s in config file: %w", f.Name, err)
			}
		}
	})

	return firstErr
}

func setFromEnv(flags *pflag.FlagSet, f *pflag.Flag) error {
	if f == nil || f.Changed {
		return nil
	}
	name := envVarName(f.Name)
	value, ok := os.LookupEnv(name)
	if !ok {
		return nil
	}
	if err := flags.Set(f.Name, value); err != nil {
		return fmt.Errorf("invalid %s: %w", name, err)
	}

	return nil
}
