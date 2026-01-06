package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

func Load[T any](configPath string) (*T, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg T
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
