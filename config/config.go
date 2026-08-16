package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config map[string]string

func getConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	
	dir := filepath.Join(home, ".config", "gatekey-proxy")
	// 0700 ensures only the owner can traverse or read this directory
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	return filepath.Join(dir, "config.json"), nil
}

func LoadConfig() (Config, error) {
	path, err := getConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(Config), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

func SaveConfig(cfg Config) error {
	path, err := getConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	// 0600 is CRITICAL for security (read/write only by the owner).
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

func GetKey(provider string) (string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return "", err
	}
	
	key, ok := cfg[provider]
	if !ok {
		return "", fmt.Errorf("key for provider '%s' not found", provider)
	}
	
	return key, nil
}

func SetKey(provider, key string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	
	cfg[provider] = key
	return SaveConfig(cfg)
}

func DeleteKey(provider string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	delete(cfg, provider)
	return SaveConfig(cfg)
}

func GetAllProviders() ([]string, error) {
	cfg, err := LoadConfig()
	if err != nil {
		return nil, err
	}
	providers := make([]string, 0, len(cfg))
	for k := range cfg {
		providers = append(providers, k)
	}
	return providers, nil
}
