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
