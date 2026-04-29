package searchplugin

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ConfigFile struct {
	Plugins []PluginConfig `yaml:"plugins"`
}

type PluginConfig struct {
	Name     string   `yaml:"name"`
	URI      string   `yaml:"uri"`
	AuthType AuthType `yaml:"auth_type"`
	Enabled  bool     `yaml:"enabled"`
}

func ReadConfigFile(configPath string) (ConfigFile, error) {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return ConfigFile{}, ErrConfigPathNotSet
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		return ConfigFile{}, err
	}

	var config ConfigFile
	if err := yaml.Unmarshal(content, &config); err != nil {
		return ConfigFile{}, fmt.Errorf("decode plugin config yaml: %w", err)
	}

	if config.Plugins == nil {
		config.Plugins = []PluginConfig{}
	}
	return config, nil
}
