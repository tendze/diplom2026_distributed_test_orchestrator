package config

type AgentConfig struct {
	Agent AgentSectionLocal `yaml:"agent"`
}

type AgentSectionLocal struct {
	Listen string `yaml:"listen"`
}
