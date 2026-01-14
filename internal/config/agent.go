package config

type AgentConfig struct {
	Agent AgentSectionLocal `yaml:"agent"`
}

type AgentSectionLocal struct {
	Listen string `yaml:"listen"`
}

func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		Agent: AgentSectionLocal{
			Listen: ":9000",
		},
	}
}
