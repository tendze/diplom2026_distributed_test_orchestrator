package config

type ControllerConfig struct {
	Agents AgentsSection `yaml:"agents"`
	Test   TestSection   `yaml:"test"`
}

type AgentsSection struct {
	Mode    string   `yaml:"mode"` // strict | any
	Targets []string `yaml:"targets"`
}

type TestSection struct {
	ID              string `yaml:"id"`
	URL             string `yaml:"url"`
	TargetRPS       int32  `yaml:"target_rps"`
	DurationSeconds int32  `yaml:"duration_seconds"`
}
