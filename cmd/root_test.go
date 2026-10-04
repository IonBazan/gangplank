package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConfig_Precedence(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
ttl: 30m
gateway: http://192.168.1.1:5000/rootDesc.xml
localIp: 192.168.1.10
refreshInterval: 5m
ports:
  - externalPort: 8080
    internalPort: 80
    protocol: tcp
    name: web
`), 0o600))

	configFile = path
	t.Cleanup(func() { configFile = "" })
	// Environment wins over the config file.
	t.Setenv("GANGPLANK_LOCAL_IP", "192.168.1.20")

	initConfig()

	require.NotNil(t, cfg, "config file must be loaded into the package-level config")
	require.Len(t, cfg.Ports, 1)
	assert.Equal(t, 8080, cfg.Ports[0].ExternalPort)

	assert.Equal(t, 30*time.Minute, ttl)
	assert.Equal(t, "http://192.168.1.1:5000/rootDesc.xml", gateway)
	assert.Equal(t, "192.168.1.20", localIP)
	assert.Equal(t, 5*time.Minute, refreshInterval)
}

func TestEnvVarName(t *testing.T) {
	assert.Equal(t, "GANGPLANK_REFRESH_INTERVAL", envVarName("refresh-interval"))
	assert.Equal(t, "GANGPLANK_DRY_RUN", envVarName("dry-run"))
}
