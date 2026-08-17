package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type UpdatePreferences struct {
	AutoCheck   bool `json:"autoCheck"`
	AutoInstall bool `json:"autoInstall"`
}

func DefaultUpdatePreferences() UpdatePreferences {
	return UpdatePreferences{AutoCheck: true}
}

func LoadUpdatePreferences() (UpdatePreferences, error) {
	dir, err := ConfigDir()
	if err != nil {
		return UpdatePreferences{}, err
	}
	path := filepath.Join(dir, "updates.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultUpdatePreferences(), nil
		}
		return UpdatePreferences{}, fmt.Errorf("failed to read update settings: %w", err)
	}

	var preferences UpdatePreferences
	if err := json.Unmarshal(data, &preferences); err != nil {
		return UpdatePreferences{}, fmt.Errorf("failed to parse update settings: %w", err)
	}
	if !preferences.AutoCheck {
		preferences.AutoInstall = false
	}
	return preferences, nil
}

func SaveUpdatePreferences(preferences UpdatePreferences) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode update settings: %w", err)
	}
	path := filepath.Join(dir, "updates.json")
	temp, err := os.CreateTemp(dir, "updates-*.json")
	if err != nil {
		return fmt.Errorf("failed to create update settings: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0600); err != nil {
		temp.Close()
		return fmt.Errorf("failed to secure update settings: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("failed to write update settings: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("failed to sync update settings: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to close update settings: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to write update settings: %w", err)
	}
	return nil
}
