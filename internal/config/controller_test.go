package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
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

func TestValidateControllerConfig(t *testing.T) {

	validConfig := func() ControllerConfig {
		return ControllerConfig{
			Agents: AgentsSection{
				Mode:    lo.ToPtr("strict"),
				Targets: []string{"agent1"},
			},
			Test: TestSection{
				ID:              "test-1",
				URL:             "http://example.com",
				TargetRPS:       10,
				DurationSeconds: 10,
			},
		}
	}

	tests := []struct {
		name    string
		modify  func(*ControllerConfig)
		wantErr error
	}{
		{
			name: "valid config",
			modify: func(c *ControllerConfig) {
			},
			wantErr: nil,
		},
		{
			name: "invalid mode",
			modify: func(c *ControllerConfig) {
				c.Agents.Mode = lo.ToPtr("wrong")
			},
			wantErr: InvalidModeError,
		},
		{
			name: "missing mode",
			modify: func(c *ControllerConfig) {
				c.Agents.Mode = nil
			},
			wantErr: nil,
		},
		{
			name: "missing ulr",
			modify: func(c *ControllerConfig) {
				c.Test.URL = ""
			},
			wantErr: MissingURLError,
		},
		{
			name: "invalid url",
			modify: func(c *ControllerConfig) {
				c.Test.URL = ":::bad_url"
			},
			wantErr: InvalidURL,
		},
		{
			name: "invalid rps",
			modify: func(c *ControllerConfig) {
				c.Test.TargetRPS = 0
			},
			wantErr: InvalidRPSValueError,
		},
		{
			name: "invalid duration",
			modify: func(c *ControllerConfig) {
				c.Test.DurationSeconds = 1
			},
			wantErr: InvalidDurationValueError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(&cfg)

			err := cfg.Validate()

			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
			}
		})
	}
}
