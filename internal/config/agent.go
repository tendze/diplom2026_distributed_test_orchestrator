package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

var (
	MissingURLError = errors.New("empty url")
	InvalidURL      = errors.New("invalid listen address")
)

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

func (c *AgentConfig) Validate() error {
	if err := validateURL(c.Agent.Listen); err != nil {
		return err
	}
	
	return nil
}

func validateURL(targetURL string) error {
	addr := strings.TrimSpace(targetURL)
	if addr == "" {
		return MissingURLError
	}

	// Кейс ":9000"
	if strings.HasPrefix(addr, ":") {
		if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
			return fmt.Errorf("%w: %w", InvalidURL, err)
		}
		return nil
	}

	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}

	u, err := url.Parse(addr)
	if err != nil || u.Host == "" {
		return InvalidURL
	}

	return nil
}
