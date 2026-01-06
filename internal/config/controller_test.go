package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadControllerConfig(t *testing.T) {
	yamlData := `
agents:
  mode: strict
  targets:
    - "localhost:9100"
    - "tbank.ru/host-machine/agent1"
test:
  id: "testnum1"
  url: "http://example.com"
  target_rps: 100
  duration_seconds: 10
`

	tmp := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(tmp, []byte(yamlData), 0644))

	cfg, err := Load[ControllerConfig](tmp)
	require.NoError(t, err)

	require.Len(t, cfg.Agents.Targets, 2)
	require.Equal(t, "testnum1", cfg.Test.ID)
	require.Equal(t, "localhost:9100", cfg.Agents.Targets[0])
	require.Equal(t, "tbank.ru/host-machine/agent1", cfg.Agents.Targets[1])
	require.Equal(t, int32(100), cfg.Test.TargetRPS)
}
