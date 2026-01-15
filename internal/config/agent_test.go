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

func TestValidateAgentConfig(t *testing.T) {
	validConfig := func() AgentConfig {
		return AgentConfig{
			Agent: AgentSectionLocal{
				Listen: "http://example.com",
			},
		}
	}

	tests := []struct {
		name    string
		modify  func(a *AgentConfig)
		wantErr error
	}{
		{
			name: "valid",
			modify: func(a *AgentConfig) {
			},
			wantErr: nil,
		},
		{
			name: "missing url",
			modify: func(a *AgentConfig) {
				a.Agent.Listen = ""
			},
			wantErr: MissingURLError,
		},
		{
			name: "invalid url",
			modify: func(a *AgentConfig) {
				a.Agent.Listen = "::::verybadhost"
			},
			wantErr: InvalidURL,
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
