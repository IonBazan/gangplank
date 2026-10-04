package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/IonBazan/gangplank/internal/types"
)

func TestLoadConfig(t *testing.T) {
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

	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, &Config{
		Ttl:             30 * time.Minute,
		Gateway:         "http://192.168.1.1:5000/rootDesc.xml",
		LocalIP:         "192.168.1.10",
		RefreshInterval: 5 * time.Minute,
		Ports:           []types.PortMapping{{ExternalPort: 8080, InternalPort: 80, Protocol: "TCP", Name: "web"}},
	}, cfg)
}

func TestLoadConfig_Example(t *testing.T) {
	cfg, err := LoadConfig("../../config.example.yaml")
	require.NoError(t, err)
	assert.Equal(t, 60*time.Minute, cfg.Ttl)
	assert.NotEmpty(t, cfg.Ports)
}

func TestLoadConfig_Missing(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	assert.Error(t, err)
}

func TestLoadConfig_DefaultLocation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ttl: 5m\n"), 0o600))
	t.Chdir(dir)

	cfg, err := LoadConfig("")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Minute, cfg.Ttl)
}

func TestLoadConfig_DefaultLocationMissing(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := LoadConfig("")
	assert.Error(t, err)
}

func TestLoadConfig_Invalid(t *testing.T) {
	tests := map[string]string{
		"Malformed YAML": "ports: [\n",
		"Wrong type":     "ports: not-a-list\n",
		"Bad duration":   "ttl: forever\n",
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

			_, err := LoadConfig(path)
			assert.Error(t, err)
		})
	}
}
