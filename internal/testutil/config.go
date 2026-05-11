package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ExtensionsIntegrationConfigFileEnvVar = "GOEXT_TEST_CONFIG_ENV"

type ExtensionsTestConfig struct {
	IrodsHost                  string `yaml:"IrodsHost"`
	IrodsPort                  int    `yaml:"IrodsPort"`
	IrodsZone                  string `yaml:"IrodsZone"`
	IrodsAdminUser             string `yaml:"IrodsAdminUser"`
	IrodsAdminPassword         string `yaml:"IrodsAdminPassword"`
	IrodsAdminPasswordFile     string `yaml:"IrodsAdminPasswordFile"`
	IrodsAuthScheme            string `yaml:"IrodsAuthScheme"`
	IrodsDefaultResource       string `yaml:"IrodsDefaultResource"`
	IrodsPrimaryTestUser       string `yaml:"IrodsPrimaryTestUser"`
	IrodsPrimaryTestPassword   string `yaml:"IrodsPrimaryTestPassword"`
	IrodsSecondaryTestUser     string `yaml:"IrodsSecondaryTestUser"`
	IrodsSecondaryTestPassword string `yaml:"IrodsSecondaryTestPassword"`
}

func ReadExtensionsTestConfig(configFile string) (*ExtensionsTestConfig, error) {
	configFile = strings.TrimSpace(configFile)
	if configFile == "" {
		return nil, fmt.Errorf("empty config file path")
	}

	content, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("read config file %q: %w", configFile, err)
	}

	cfg := &ExtensionsTestConfig{}
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return nil, fmt.Errorf("parse config file %q: %w", configFile, err)
	}

	configDir := filepath.Dir(configFile)
	if cfg.IrodsAdminPassword == "" && strings.TrimSpace(cfg.IrodsAdminPasswordFile) != "" {
		passwordFile := strings.TrimSpace(cfg.IrodsAdminPasswordFile)
		if !filepath.IsAbs(passwordFile) {
			passwordFile = filepath.Join(configDir, passwordFile)
		}

		passwordBytes, err := os.ReadFile(passwordFile)
		if err != nil {
			return nil, fmt.Errorf("read iRODS admin password file %q: %w", passwordFile, err)
		}

		cfg.IrodsAdminPassword = strings.TrimSpace(string(passwordBytes))
	}

	return cfg, nil
}
