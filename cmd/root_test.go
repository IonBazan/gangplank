package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errStop = errors.New("stop")

// settingsOf runs the daemon command until it needs Docker and returns the resolved settings.
func settingsOf(t *testing.T, args ...string) (*app, error) {
	t.Helper()
	a := newApp()
	a.newDocker = func() (*client.Client, error) { return nil, errStop }

	_, err := run(t, context.Background(), a, append([]string{"daemon"}, args...)...)
	if errors.Is(err, errStop) {
		err = nil
	}
	return a, err
}

const fullConfig = `
ttl: 30m
gateway: http://192.168.1.1:5000/rootDesc.xml
localIp: 192.168.1.10
refreshInterval: 5m
ports:
  - externalPort: 8080
    internalPort: 80
    protocol: tcp
    name: web
`

func TestSettings_Precedence(t *testing.T) {
	config := writeConfig(t, fullConfig)
	t.Setenv("GANGPLANK_LOCAL_IP", "192.168.1.20")
	t.Setenv("GANGPLANK_TTL", "20m")

	a, err := settingsOf(t, "--config", config, "--ttl", "45m")
	require.NoError(t, err)

	require.NotNil(t, a.cfg)
	require.Len(t, a.cfg.Ports, 1)
	assert.Equal(t, 8080, a.cfg.Ports[0].ExternalPort)

	assert.Equal(t, 45*time.Minute, a.opts.ttl, "flag wins over env and file")
	assert.Equal(t, "192.168.1.20", a.opts.localIP, "env wins over file")
	assert.Equal(t, "http://192.168.1.1:5000/rootDesc.xml", a.opts.gateway, "file wins over default")
	assert.Equal(t, 5*time.Minute, a.opts.refreshInterval)
}

func TestSettings_Defaults(t *testing.T) {
	t.Chdir(t.TempDir())

	a, err := settingsOf(t)
	require.NoError(t, err)

	assert.Nil(t, a.cfg)
	assert.Equal(t, time.Hour, a.opts.ttl)
	assert.Equal(t, 15*time.Minute, a.opts.refreshInterval)
	assert.False(t, a.opts.poll)
}

func TestSettings_DefaultConfigFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeConfigAt(t, dir+"/config.yaml", "ttl: 2h\n")

	a, err := settingsOf(t)
	require.NoError(t, err)
	assert.Equal(t, 2*time.Hour, a.opts.ttl)
}

func TestSettings_ConfigFileFromEnvironment(t *testing.T) {
	t.Setenv("GANGPLANK_CONFIG", writeConfig(t, fullConfig))
	t.Setenv("GANGPLANK_POLL", "true")

	a, err := settingsOf(t)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, a.opts.ttl)
	assert.True(t, a.opts.poll)
}

func TestSettings_Errors(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		config  string
		args    []string
		wantErr string
	}{
		{name: "Invalid environment value", env: map[string]string{"GANGPLANK_TTL": "soon"}, wantErr: "invalid GANGPLANK_TTL"},
		{name: "Missing config file", args: []string{"--config", "/does/not/exist.yaml"}, wantErr: "error loading config file"},
		{name: "Unknown config key", config: "duration: 60m\n", wantErr: "field duration not found"},
		{name: "Invalid config value", config: "ttl: soon\n", wantErr: "error loading config file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			args := tt.args
			if tt.config != "" {
				args = append(args, "--config", writeConfig(t, tt.config))
			}

			_, err := settingsOf(t, args...)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestEnvVarName(t *testing.T) {
	assert.Equal(t, "GANGPLANK_REFRESH_INTERVAL", envVarName("refresh-interval"))
	assert.Equal(t, "GANGPLANK_DRY_RUN", envVarName("dry-run"))
}

func TestLogging(t *testing.T) {
	tests := []struct {
		args    []string
		wantErr string
	}{
		{args: []string{"--log-level", "debug", "--log-format", "json"}},
		{args: []string{"--log-level", "WARN"}},
		{args: []string{"--log-level", "loud"}, wantErr: `invalid log level "loud"`},
		{args: []string{"--log-format", "xml"}, wantErr: `invalid log format "xml"`},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			_, err := settingsOf(t, tt.args...)
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestLogging_WritesToStderr(t *testing.T) {
	a := newApp()
	a.newDocker = func() (*client.Client, error) { return nil, errStop }

	var out, logs bytes.Buffer
	root := a.rootCmd()
	root.SetOut(&out)
	root.SetErr(&logs)
	root.SetArgs([]string{"daemon", "--log-format", "json"})
	assert.ErrorIs(t, root.Execute(), errStop)

	assert.Empty(t, out.String())
	assert.Contains(t, logs.String(), banner, "the banner is shown for the daemon")
	assert.Contains(t, logs.String(), `"msg":"Starting Gangplank daemon"`)
}

func TestBannerOnlyForDaemon(t *testing.T) {
	a := newApp()
	var out, logs bytes.Buffer
	root := a.rootCmd()
	root.SetOut(&out)
	root.SetErr(&logs)
	root.SetArgs([]string{"list", "--dry-run"})
	require.NoError(t, root.Execute())

	assert.NotContains(t, logs.String(), banner)
}
