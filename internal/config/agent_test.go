package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadAgentConfig(t *testing.T) {
	yamlData := `
agent:
 listen: ":9001"
`

	tmp := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(tmp, []byte(yamlData), 0644))

	cfg, err := Load[AgentConfig](tmp)
	require.NoError(t, err)

	require.Equal(t, ":9001", cfg.Agent.Listen)
}
