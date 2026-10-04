package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/upnp"
	"github.com/IonBazan/gangplank/internal/upnp/upnptest"
)

type fakeDocker struct {
	containers string // JSON body of /containers/json
	inspect    map[string]string
	events     chan string
}

func newFakeDocker(t *testing.T, containers string) *fakeDocker {
	t.Helper()
	d := &fakeDocker{containers: containers, inspect: map[string]string{}, events: make(chan string, 10)}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Api-Version", "1.45")
		w.Header().Set("Content-Type", "application/json")
		switch path := r.URL.Path; {
		case strings.HasSuffix(path, "/_ping"):
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(path, "/containers/json"):
			_, _ = w.Write([]byte(d.containers))
		case strings.HasSuffix(path, "/events"):
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			for {
				select {
				case event := <-d.events:
					_, _ = w.Write([]byte(event + "\n"))
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		case strings.Contains(path, "/containers/") && strings.HasSuffix(path, "/json"):
			id := strings.TrimSuffix(path[strings.Index(path, "/containers/")+len("/containers/"):], "/json")
			body, ok := d.inspect[id]
			if !ok {
				http.Error(w, `{"message":"no such container"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(body))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	useDocker(t, "tcp://"+srv.Listener.Addr().String())
	return d
}

func useDocker(t *testing.T, host string) {
	t.Helper()
	original := NewDockerClient
	NewDockerClient = func() (*client.Client, error) {
		return client.New(client.WithHost(host))
	}
	t.Cleanup(func() { NewDockerClient = original })
}

func resetFlags(t *testing.T) {
	t.Helper()
	reset := func(flags *pflag.FlagSet) {
		flags.VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	}
	cleanup := func() {
		viper.Reset()
		cfg = nil
		reset(rootCmd.PersistentFlags())
		for _, c := range rootCmd.Commands() {
			reset(c.Flags())
		}
	}
	cleanup()
	t.Cleanup(cleanup)
}

func run(t *testing.T, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	resetFlags(t)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})

	// Cobra only sets a subcommand's context when it has none, so later runs
	// would otherwise inherit the (cancelled) context of an earlier one.
	for _, c := range rootCmd.Commands() {
		c.SetContext(ctx)
	}

	_, err := rootCmd.ExecuteContextC(ctx)
	return out.String(), err
}

func gatewayArgs(igd *upnptest.IGD) []string {
	return []string{"--gateway", igd.URL(), "--local-ip", "192.168.1.10"}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestAddCommand(t *testing.T) {
	igd := upnptest.NewIGD(t)

	_, err := run(t, context.Background(), append([]string{"add", "8080:80/udp", "--name", "web"}, gatewayArgs(igd)...)...)
	require.NoError(t, err)
	assert.Equal(t, []upnptest.Mapping{
		{ExternalPort: 8080, InternalPort: 80, Protocol: "UDP", InternalIP: "192.168.1.10", Description: "Gangplank UPnP: web", Lease: 3600},
	}, igd.Mappings())
}

func TestAddCommand_Errors(t *testing.T) {
	igd := upnptest.NewIGD(t)
	igd.PermanentOnly = true

	_, err := run(t, context.Background(), append([]string{"add", "99999"}, gatewayArgs(igd)...)...)
	assert.ErrorContains(t, err, "failed to parse port mapping")

	_, err = run(t, context.Background(), "add", "80", "--gateway", "http://127.0.0.1:1/rootDesc.xml")
	assert.ErrorContains(t, err, "failed to initialize UPnP client")

	_, err = run(t, context.Background(), "add")
	assert.Error(t, err, "argument is required")
}

func TestDeleteCommand(t *testing.T) {
	igd := upnptest.NewIGD(t)
	igd.Add(upnptest.Mapping{ExternalPort: 25565, InternalPort: 25565, Protocol: "TCP", InternalIP: "192.168.1.10"})

	_, err := run(t, context.Background(), append([]string{"delete", "25565/tcp"}, gatewayArgs(igd)...)...)
	require.NoError(t, err)
	assert.Empty(t, igd.Mappings())

	_, err = run(t, context.Background(), append([]string{"delete", "25565/tcp"}, gatewayArgs(igd)...)...)
	assert.ErrorContains(t, err, "failed to delete port mapping 25565/TCP")

	_, err = run(t, context.Background(), append([]string{"delete", "x"}, gatewayArgs(igd)...)...)
	assert.ErrorContains(t, err, "failed to parse port mapping")
}

func TestListCommand(t *testing.T) {
	igd := upnptest.NewIGD(t)

	out, err := run(t, context.Background(), append([]string{"list"}, gatewayArgs(igd)...)...)
	require.NoError(t, err)
	assert.Empty(t, out, "nothing is printed to stdout without mappings")

	igd.Add(upnptest.Mapping{ExternalPort: 443, InternalPort: 8443, Protocol: "TCP", InternalIP: "192.168.1.10", Description: "Gangplank UPnP: web", Lease: 3600})
	igd.Add(upnptest.Mapping{ExternalPort: 53, InternalPort: 53, Protocol: "UDP", InternalIP: "192.168.1.11", Description: "dns"})

	out, err = run(t, context.Background(), append([]string{"list"}, gatewayArgs(igd)...)...)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 4)
	assert.Contains(t, lines[0], "External Port")
	assert.Equal(t, []string{"443", "8443", "TCP", "192.168.1.10", "Gangplank", "UPnP:", "web", "3600", "seconds", "true"}, strings.Fields(lines[2]))
	assert.Equal(t, []string{"53", "53", "UDP", "192.168.1.11", "dns", "Permanent", "true"}, strings.Fields(lines[3]))
}

const webContainer = `[{"Id":"web1234567890","Names":["/web"],"Labels":{"gangplank.forward":"published"},
	"Ports":[{"IP":"0.0.0.0","PrivatePort":80,"PublicPort":8080,"Type":"tcp"},{"IP":"::","PrivatePort":80,"PublicPort":8080,"Type":"tcp"}]}]`

func TestForwardCommand(t *testing.T) {
	igd := upnptest.NewIGD(t)
	newFakeDocker(t, webContainer)
	config := writeConfig(t, "ports:\n  - externalPort: 53\n    internalPort: 53\n    protocol: udp\n    name: dns\n")

	_, err := run(t, context.Background(), append([]string{"forward", "--config", config}, gatewayArgs(igd)...)...)
	require.NoError(t, err)
	assert.Equal(t, []upnptest.Mapping{
		{ExternalPort: 8080, InternalPort: 8080, Protocol: "TCP", InternalIP: "192.168.1.10", Description: "Gangplank UPnP: web", Lease: 3600},
		{ExternalPort: 53, InternalPort: 53, Protocol: "UDP", InternalIP: "192.168.1.10", Description: "Gangplank UPnP: dns", Lease: 3600},
	}, igd.Mappings())
}

func TestForwardCommand_DockerUnavailable(t *testing.T) {
	igd := upnptest.NewIGD(t)
	useDocker(t, "tcp://127.0.0.1:1")
	config := writeConfig(t, "ports:\n  - externalPort: 53\n    internalPort: 53\n")

	_, err := run(t, context.Background(), append([]string{"forward", "--config", config}, gatewayArgs(igd)...)...)

	// The command fails, but the static ports are still forwarded.
	assert.ErrorContains(t, err, "failed to list containers")
	assert.Len(t, igd.Mappings(), 1)
}

func TestForwardCommand_GatewayUnavailable(t *testing.T) {
	newFakeDocker(t, "[]")

	_, err := run(t, context.Background(), "forward", "--gateway", "http://127.0.0.1:1/rootDesc.xml")
	assert.ErrorContains(t, err, "failed to initialize UPnP client")
}

func TestForwardCommand_InvalidConfigFile(t *testing.T) {
	newFakeDocker(t, "[]")
	config := writeConfig(t, "ports:\n  - externalPort: 53\n    internalPort: 0\n")

	_, err := run(t, context.Background(), "forward", "--dry-run", "--config", config)
	assert.ErrorContains(t, err, "invalid port mapping at index 0")
}

func runDaemon(t *testing.T, args ...string) func() error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := run(t, ctx, append([]string{"daemon"}, args...)...)
		errCh <- err
	}()

	return func() error {
		cancel()
		select {
		case err := <-errCh:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("daemon did not stop")
			return nil
		}
	}
}

func TestDaemonCommand(t *testing.T) {
	igd := upnptest.NewIGD(t)
	newFakeDocker(t, webContainer)
	// A stale mapping from an earlier run, and one owned by something else.
	igd.Add(upnptest.Mapping{ExternalPort: 9000, InternalPort: 9000, Protocol: "TCP", InternalIP: "192.168.1.10", Description: "Gangplank UPnP: removed"})
	igd.Add(upnptest.Mapping{ExternalPort: 32400, InternalPort: 32400, Protocol: "TCP", InternalIP: "192.168.1.20", Description: "Plex"})

	stop := runDaemon(t, append([]string{"--prune", "--cleanup-on-exit", "--refresh-interval", "20ms"}, gatewayArgs(igd)...)...)

	assert.Eventually(t, func() bool {
		m := igd.Mappings()
		return len(m) == 2 && m[0].ExternalPort == 8080 && m[1].ExternalPort == 32400
	}, 5*time.Second, 10*time.Millisecond, "stale mapping should be pruned and the container forwarded")

	require.NoError(t, stop())
	assert.Equal(t, []upnptest.Mapping{
		{ExternalPort: 32400, InternalPort: 32400, Protocol: "TCP", InternalIP: "192.168.1.20", Description: "Plex"},
	}, igd.Mappings(), "only our mappings are removed on exit")
}

func TestDaemonCommand_Poll(t *testing.T) {
	igd := upnptest.NewIGD(t)
	docker := newFakeDocker(t, "[]")
	docker.inspect["game1234567890"] = `{"Id":"game1234567890","Name":"/game","Config":{"Labels":{"gangplank.forward":"published"}},
		"NetworkSettings":{"Ports":{"25565/tcp":[{"HostIp":"0.0.0.0","HostPort":"25565"}]}}}`

	stop := runDaemon(t, append([]string{"--poll", "--cleanup-on-stop"}, gatewayArgs(igd)...)...)

	docker.events <- `{"Type":"container","Action":"start","Actor":{"ID":"game1234567890"}}`
	assert.Eventually(t, func() bool { return len(igd.Mappings()) == 1 }, 5*time.Second, 10*time.Millisecond)

	docker.events <- `{"Type":"container","Action":"die","Actor":{"ID":"game1234567890"}}`
	assert.Eventually(t, func() bool { return len(igd.Mappings()) == 0 }, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, stop())
}

func TestDaemonCommand_RetriesGateway(t *testing.T) {
	newFakeDocker(t, webContainer)
	conn := &upnp.DummyConnection{}

	var attempts atomic.Int32
	original := SetupUPnPClient
	SetupUPnPClient = func(ctx context.Context) (*upnp.Client, error) {
		if attempts.Add(1) < 3 {
			return nil, errors.New("router is booting")
		}
		return upnp.NewClientWithConnection(conn, "192.168.1.10", time.Hour), nil
	}
	t.Cleanup(func() { SetupUPnPClient = original })

	stop := runDaemon(t, "--refresh-interval", "10ms")
	assert.Eventually(t, func() bool {
		forwarded, _ := conn.Snapshot()
		return len(forwarded) > 0
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, stop())
	assert.GreaterOrEqual(t, attempts.Load(), int32(3))
}

func TestDaemonCommand_InvalidRefreshInterval(t *testing.T) {
	_, err := run(t, context.Background(), "daemon", "--refresh-interval", "0s")
	assert.ErrorContains(t, err, "--refresh-interval must be greater than 0")
}

func TestDaemonCommand_EnvironmentVariables(t *testing.T) {
	newFakeDocker(t, "[]")
	t.Setenv("GANGPLANK_REFRESH_INTERVAL", "-1s")

	_, err := run(t, context.Background(), "daemon", "--dry-run")
	assert.ErrorContains(t, err, "--refresh-interval must be greater than 0", "environment variable should set the flag")
}

func TestCommandsRejectExtraArguments(t *testing.T) {
	for _, args := range [][]string{{"list", "x"}, {"forward", "x"}, {"daemon", "x"}, {"add", "1", "2"}, {"delete", "1", "2"}} {
		_, err := run(t, context.Background(), args...)
		assert.Error(t, err, "%v", args)
	}
}
