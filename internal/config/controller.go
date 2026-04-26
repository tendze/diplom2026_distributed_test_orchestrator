package config

import "errors"

var (
	InvalidModeError                 = errors.New("mode must be one of \"strict\" or \"any\"")
	InvalidDistributionError         = errors.New("distribution mode must be one of \"equal\" or \"adaptive\"")
	InvalidRPSValueError             = errors.New("target rps must be > 0")
	InvalidDurationValueError        = errors.New("test duration must be > 1 sec")
	InvalidMissingModeError          = errors.New("missing mode for distributed test")
	InvalidNegativeWorkersCountError = errors.New("workers count cant be negative")
)

type ControllerConfig struct {
	Agents AgentsSection `yaml:"agents"`
	Test   TestSection   `yaml:"test"`
}

type AgentsSection struct {
	Mode             *string  `yaml:"mode"`              // strict | any
	DistributionMode *string  `yaml:"distribution_mode"` // equal | adaptive
	Targets          []string `yaml:"targets"`
	// TODO: add distribution_mode -  equal/adaptive
}

type TestSection struct {
	ID              string          `yaml:"id"`
	URL             string          `yaml:"url"`
	TargetRPS       int32           `yaml:"target_rps"`
	DurationSeconds int32           `yaml:"duration_seconds"`
	Workers         int32           `yaml:"workers"`
	Monitor         *MonitorSection `yaml:"monitor"`
}

type MonitorSection struct {
	Enabled *bool `yaml:"enabled"`
}

func (cc *ControllerConfig) Validate() error {
	if cc.Agents.Mode != nil && *cc.Agents.Mode != "strict" && *cc.Agents.Mode != "any" {
		return InvalidModeError
	}
	if cc.Agents.DistributionMode != nil && *cc.Agents.DistributionMode != "adaptive" && *cc.Agents.DistributionMode != "equal" {
		return InvalidDistributionError
	}
	if len(cc.Agents.Targets) != 0 && cc.Agents.Mode == nil {
		return InvalidMissingModeError
	}
	if err := validateURL(cc.Test.URL); err != nil {
		return err
	}
	if cc.Test.TargetRPS <= 0 {
		return InvalidRPSValueError
	}
	if cc.Test.DurationSeconds <= 1 {
		return InvalidDurationValueError
	}
	if cc.Test.Workers < 0 {
		return InvalidNegativeWorkersCountError
	}
	return nil
}
