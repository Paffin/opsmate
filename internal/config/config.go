package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

type Config struct {
	Servers ServerConfigs `mapstructure:"servers" yaml:"servers"`
	Safety  SafetyConfig  `mapstructure:"safety" yaml:"safety"`
	Claude  ClaudeConfig  `mapstructure:"claude" yaml:"claude"`
}

type ServerConfigs struct {
	Kubernetes KubernetesConfig `mapstructure:"kubernetes" yaml:"kubernetes"`
	Docker     DockerConfig     `mapstructure:"docker" yaml:"docker"`
	Prometheus PrometheusConfig `mapstructure:"prometheus" yaml:"prometheus"`
	Files      FilesConfig      `mapstructure:"files" yaml:"files"`
}

type KubernetesConfig struct {
	Enabled    bool     `mapstructure:"enabled" yaml:"enabled"`
	Kubeconfig string   `mapstructure:"kubeconfig" yaml:"kubeconfig"`
	Context    string   `mapstructure:"context" yaml:"context"`
	Namespaces []string `mapstructure:"namespaces" yaml:"namespaces"`
	ReadOnly   bool     `mapstructure:"readonly" yaml:"readonly"`
}

type DockerConfig struct {
	Enabled  bool   `mapstructure:"enabled" yaml:"enabled"`
	Host     string `mapstructure:"host" yaml:"host"`
	ReadOnly bool   `mapstructure:"readonly" yaml:"readonly"`
}

type PrometheusConfig struct {
	Enabled   bool       `mapstructure:"enabled" yaml:"enabled"`
	URL       string     `mapstructure:"url" yaml:"url"`
	BasicAuth *BasicAuth `mapstructure:"basicAuth" yaml:"basicAuth,omitempty"`
}

type BasicAuth struct {
	Username string `mapstructure:"username" yaml:"username"`
	Password string `mapstructure:"password" yaml:"password"`
}

type FilesConfig struct {
	Enabled   bool     `mapstructure:"enabled" yaml:"enabled"`
	ScanPaths []string `mapstructure:"scan_paths" yaml:"scan_paths"`
	Rulesets  []string `mapstructure:"rulesets" yaml:"rulesets"`
}

type SafetyConfig struct {
	ConfirmDestructive bool `mapstructure:"confirm_destructive" yaml:"confirm_destructive"`
	MaxLogLines        int  `mapstructure:"max_log_lines" yaml:"max_log_lines"`
	RedactSecrets      bool `mapstructure:"redact_secrets" yaml:"redact_secrets"`
}

type ClaudeConfig struct {
	Model        string `mapstructure:"model" yaml:"model"`
	CustomPrompt string `mapstructure:"custom_prompt" yaml:"custom_prompt"`
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		Servers: ServerConfigs{
			Kubernetes: KubernetesConfig{
				Enabled:    true,
				Kubeconfig: defaultKubeconfig(),
				Namespaces: []string{},
				ReadOnly:   false,
			},
			Docker: DockerConfig{
				Enabled:  true,
				Host:     defaultDockerHost(),
				ReadOnly: true,
			},
			Prometheus: PrometheusConfig{
				Enabled: false,
				URL:     "http://localhost:9090",
			},
			Files: FilesConfig{
				Enabled:   true,
				ScanPaths: []string{"."},
				Rulesets:  []string{"dockerfile", "kubernetes", "compose", "terraform"},
			},
		},
		Safety: SafetyConfig{
			ConfirmDestructive: true,
			MaxLogLines:        1000,
			RedactSecrets:      true,
		},
		Claude: ClaudeConfig{
			Model: "claude-sonnet-4-20250514",
		},
	}
}

func defaultKubeconfig() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kube", "config")
}

func defaultDockerHost() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		return host
	}
	if runtime.GOOS == "windows" {
		return "npipe:////./pipe/docker_engine"
	}
	return "unix:///var/run/docker.sock"
}

// Load reads the configuration from file and environment.
func Load(cfgFile string) (*Config, error) {
	cfg := DefaultConfig()

	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return cfg, nil // return defaults if can't find home
		}
		viper.AddConfigPath(filepath.Join(home, ".opsmate"))
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
	}

	viper.SetEnvPrefix("OPSMATE")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			return cfg, nil // config file not found, use defaults
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}

// ConfigDir returns the opsmate config directory path.
func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".opsmate")
}
