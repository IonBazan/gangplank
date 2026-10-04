package cmd

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/IonBazan/gangplank/internal/config"
	"github.com/IonBazan/gangplank/internal/gangplank"
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
	created = "unknown"
)

type options struct {
	configFile string
	dryRun     bool
	localIP    string
	gateway    string
	ttl        time.Duration
	logLevel   string
	logFormat  string

	poll            bool
	cleanupOnStop   bool
	cleanupOnExit   bool
	prune           bool
	refreshInterval time.Duration
}

type app struct {
	opts options
	cfg  *config.Config

	connectGateway func(ctx context.Context) (gangplank.Gateway, error)
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

func (a *app) defaultGateway(ctx context.Context) (gangplank.Gateway, error) {
	if a.opts.dryRun {
		return upnp.NewDryRunClient(a.opts.ttl), nil
	}

	client, err := upnp.NewClient(ctx, a.opts.localIP, a.opts.gateway, a.opts.ttl)
	if err != nil {
		return nil, err
	}
	return client, nil
}

func Execute() {
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
			if err := a.loadSettings(cmd.Flags()); err != nil {
				return err
			}
			return a.setupLogging(cmd.ErrOrStderr())
		},
	}

	flags := root.PersistentFlags()
	flags.StringVarP(&a.opts.configFile, "config", "c", "", "config file path (default: ./gangplank.yaml if present)")
	flags.BoolVar(&a.opts.dryRun, "dry-run", false, "Do not apply changes - only list the ports")
	flags.StringVar(&a.opts.localIP, "local-ip", "", "Local IP address to use for UPnP (default: auto-detected)")
	flags.StringVar(&a.opts.gateway, "gateway", "", "UPnP gateway description URL, e.g. http://192.168.1.1:5000/rootDesc.xml (default: auto-detected)")
	flags.DurationVar(&a.opts.ttl, "ttl", upnp.DefaultLeaseDuration, "UPnP lease duration")
	flags.StringVar(&a.opts.logLevel, "log-level", "info", "Log level: debug, info, warn or error")
	flags.StringVar(&a.opts.logFormat, "log-format", "text", "Log format: text or json")

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
		if firstErr != nil || f.Changed || f.Annotations[cobra.FlagSetByCobraAnnotation] != nil {
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

// Logs go to stderr, so command output (e.g. list) can be piped.
func (a *app) setupLogging(w io.Writer) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(a.opts.logLevel)); err != nil {
		return fmt.Errorf("invalid log level %q: use debug, info, warn or error", a.opts.logLevel)
	}

	handlerOpts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	switch a.opts.logFormat {
	case "text":
		handler = slog.NewTextHandler(w, handlerOpts)
	case "json":
		handler = slog.NewJSONHandler(w, handlerOpts)
	default:
		return fmt.Errorf("invalid log format %q: use text or json", a.opts.logFormat)
	}
	slog.SetDefault(slog.New(handler))

	return nil
}
