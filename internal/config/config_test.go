package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/portmap"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gangplank.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
ttl: 30m
gateway: http://192.168.1.1:5000/rootDesc.xml
localIp: 192.168.1.10
refreshInterval: 5m
ports:
  - externalPort: 8080
    internalPort: 80
    protocol: TCP
    name: web
`), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, &Config{
		TTL:             new(30 * time.Minute),
		Gateway:         "http://192.168.1.1:5000/rootDesc.xml",
		LocalIP:         "192.168.1.10",
		RefreshInterval: 5 * time.Minute,
		Ports:           []portmap.Mapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}},
	}, cfg)
}

func TestLoadConfig_Example(t *testing.T) {
	cfg, err := Load("../../gangplank.example.yaml")
	require.NoError(t, err)
	assert.Equal(t, new(60*time.Minute), cfg.TTL)
	assert.NotEmpty(t, cfg.Ports)
}

func TestLoadConfig_Missing(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	assert.Error(t, err)
}

func TestLoadConfig_DefaultLocation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gangplank.yaml"), []byte("ttl: 5m\n"), 0o600))
	t.Chdir(dir)

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, new(5*time.Minute), cfg.TTL)
}

func TestLoadConfig_IgnoresGenericConfigFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("database: postgres\n"), 0o600))
	t.Chdir(dir)

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestLoadConfig_LegacyDefaultFile(t *testing.T) {
	for _, name := range []string{"config.yaml", "config.yml"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("ttl: 5m\n"), 0o600))
			t.Chdir(dir)

			_, err := Load("")
			assert.ErrorContains(t, err, name+" is no longer read by default: rename it to gangplank.yaml")
		})
	}
}

func TestLoadConfig_PrefersNewDefaultOverLegacy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ttl: 5m\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gangplank.yaml"), []byte("ttl: 2h\n"), 0o600))
	t.Chdir(dir)

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, new(2*time.Hour), cfg.TTL)
}

func TestLoadConfig_DefaultLocationMissing(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg, err := Load("")
	assert.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestLoadConfig_DefaultYmlExtension(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gangplank.yml"), []byte("gateway: http://router/desc.xml\n"), 0o600))
	t.Chdir(dir)

	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, "http://router/desc.xml", cfg.Gateway)
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gangplank.yaml")
	require.NoError(t, os.WriteFile(path, nil, 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, &Config{}, cfg)
}

func TestConfig_FlagValues(t *testing.T) {
	var empty *Config
	assert.Empty(t, empty.FlagValues())
	assert.Empty(t, (&Config{}).FlagValues())

	cfg := &Config{TTL: new(30 * time.Minute), Gateway: "http://router/desc.xml", LocalIP: "192.168.1.10", RefreshInterval: 5 * time.Minute}
	assert.Equal(t, map[string]string{
		"ttl":              "30m0s",
		"gateway":          "http://router/desc.xml",
		"local-ip":         "192.168.1.10",
		"refresh-interval": "5m0s",
	}, cfg.FlagValues())
}

func TestConfig_PermanentLease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gangplank.yaml")
	require.NoError(t, os.WriteFile(path, []byte("ttl: 0s\n"), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"ttl": "0s"}, cfg.FlagValues())
}

func TestLoadConfig_Invalid(t *testing.T) {
	tests := map[string]string{
		"Malformed YAML": "ports: [\n",
		"Wrong type":     "ports: not-a-list\n",
		"Bad duration":   "ttl: forever\n",
		"Unknown key":    "duration: 60m\n",
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gangplank.yaml")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

			_, err := Load(path)
			assert.Error(t, err)
		})
	}
}
