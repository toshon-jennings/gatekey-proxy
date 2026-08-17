package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdatePreferencesDefaultAndPersistence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	preferences, err := LoadUpdatePreferences()
	if err != nil {
		t.Fatalf("LoadUpdatePreferences() error = %v", err)
	}
	if !preferences.AutoCheck || preferences.AutoInstall {
		t.Fatalf("default preferences = %#v", preferences)
	}

	want := UpdatePreferences{AutoCheck: true, AutoInstall: true}
	if err := SaveUpdatePreferences(want); err != nil {
		t.Fatalf("SaveUpdatePreferences() error = %v", err)
	}
	got, err := LoadUpdatePreferences()
	if err != nil {
		t.Fatalf("LoadUpdatePreferences() error = %v", err)
	}
	if got != want {
		t.Fatalf("LoadUpdatePreferences() = %#v, want %#v", got, want)
	}

	info, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".config", "gatekey-proxy", "updates.json"))
	if err != nil {
		t.Fatalf("Stat(updates.json) error = %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0600 {
		t.Fatalf("updates.json mode = %o, want 600", gotMode)
	}
}

func TestLoadUpdatePreferencesDisablesInstallWithoutChecks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "gatekey-proxy")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "updates.json"), []byte(`{"autoCheck":false,"autoInstall":true}`), 0600); err != nil {
		t.Fatal(err)
	}

	preferences, err := LoadUpdatePreferences()
	if err != nil {
		t.Fatalf("LoadUpdatePreferences() error = %v", err)
	}
	if preferences.AutoCheck || preferences.AutoInstall {
		t.Fatalf("normalized preferences = %#v", preferences)
	}
}
