package runner

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/CircleCI-Public/circleci-cli/api/runner"
)

type AgentConfig struct {
	API     APIConfig     `yaml:"api"`
	Runner  RunnerConfig  `yaml:"runner"`
	Logging LoggingConfig `yaml:"logging,omitempty"`
}

func NewAgentConfig(t runner.Token, platform string) (c *AgentConfig, err error) {
	c = &AgentConfig{
		API: APIConfig{
			AuthToken: t.Token,
		},
		Runner: RunnerConfig{
			Name:                    t.Nickname,
			ResourceClass:           t.ResourceClass,
			CleanupWorkingDirectory: true,
		},
	}

	switch platform {
	default:
		return nil, fmt.Errorf("unknown platform: %q", platform)

	case "linux":
		c.Runner.CommandPrefix = []string{"/opt/circleci/launch-task"}
		c.Runner.WorkingDirectory = "/opt/circleci/workdir/%s"

	case "macos":
		c.Runner.CommandPrefix = []string{"sudo", "-niHu", "USERNAME", "--"}
		c.Runner.WorkingDirectory = "/tmp/%s"
		c.Logging.File = "/Library/Logs/com.circleci.runner.log"
	}

	return c, nil
}

func (c *AgentConfig) WriteYaml(w io.Writer) error {
	return yaml.NewEncoder(w).Encode(c)
}

type APIConfig struct {
	AuthToken string `yaml:"auth_token"`
}

type RunnerConfig struct {
	Name                    string   `yaml:"name"`
	ResourceClass           string   `yaml:"resource_class"`
	CommandPrefix           []string `yaml:"command_prefix,flow"`
	WorkingDirectory        string   `yaml:"working_directory"`
	CleanupWorkingDirectory bool     `yaml:"cleanup_working_directory"`
}

type LoggingConfig struct {
	File string `yaml:"file,omitempty"`
}
