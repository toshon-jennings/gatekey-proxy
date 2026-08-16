package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ModelSetting struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

var defaultModels = []ModelSetting{
	{Provider: "groq", Model: "llama-3.3-70b-versatile"},
	{Provider: "groq", Model: "llama-3.1-8b-instant"},
	{Provider: "openrouter", Model: "anthropic/claude-3.5-sonnet"},
	{Provider: "openai", Model: "gpt-4o"},
	{Provider: "opencode", Model: "deepseek-v4-flash"},
}

func getModelsPath() (string, error) {
	configPath, err := getConfigPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "models.json"), nil
}

func LoadModels() ([]ModelSetting, error) {
	path, err := getModelsPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return append([]ModelSetting(nil), defaultModels...), nil
		}
		return nil, fmt.Errorf("failed to read models file: %w", err)
	}

	var models []ModelSetting
	if err := json.Unmarshal(data, &models); err != nil {
		return nil, fmt.Errorf("failed to parse models file: %w", err)
	}
	if err := ValidateModels(models); err != nil {
		return nil, fmt.Errorf("invalid models file: %w", err)
	}
	return models, nil
}

func SaveModels(models []ModelSetting) error {
	if err := ValidateModels(models); err != nil {
		return err
	}

	path, err := getModelsPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode models: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write models file: %w", err)
	}
	return nil
}

func ValidateModels(models []ModelSetting) error {
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.Provider) == "" || strings.TrimSpace(model.Model) == "" {
			return fmt.Errorf("provider and model are required")
		}
		key := strings.ToLower(strings.TrimSpace(model.Provider)) + "\x00" + strings.TrimSpace(model.Model)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate model %q for provider %q", model.Model, model.Provider)
		}
		seen[key] = struct{}{}
	}
	return nil
}
