package config

import "errors"

var (
	InvalidModeError          = errors.New("mode must be one of \"strict\" or \"any\"")
	InvalidRPSValueError      = errors.New("target rps must be > 0")
	InvalidDurationValueError = errors.New("test duration must be > 1 sec")
	InvalidMissingModeError   = errors.New("missing mode for distributed test")
)

type ControllerConfig struct {
	Agents AgentsSection `yaml:"agents"`
	Test   TestSection   `yaml:"test"`
}

type AgentsSection struct {
	Mode    *string  `yaml:"mode"` // strict | any
	Targets []string `yaml:"targets"`
}

type TestSection struct {
	ID              string `yaml:"id"`
	URL             string `yaml:"url"`
	TargetRPS       int32  `yaml:"target_rps"`
	DurationSeconds int32  `yaml:"duration_seconds"`
	Workers         int32  `yaml:"workers"`
}

func (cc *ControllerConfig) Validate() error {
	if cc.Agents.Mode != nil && *cc.Agents.Mode != "strict" && *cc.Agents.Mode != "any" {
		return InvalidModeError
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

	return nil
}
