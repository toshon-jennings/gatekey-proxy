package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadModelsUsesDefaultsUntilSettingsAreSaved(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	models, err := LoadModels()
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !reflect.DeepEqual(models, defaultModels) {
		t.Fatalf("LoadModels() = %#v, want defaults %#v", models, defaultModels)
	}

	if err := SaveModels([]ModelSetting{}); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}
	models, err = LoadModels()
	if err != nil {
		t.Fatalf("LoadModels() after save error = %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("LoadModels() after empty save = %#v, want empty", models)
	}
}

func TestSaveModelsPersistsWithOwnerOnlyPermissions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want := []ModelSetting{{Provider: "openrouter", Model: "example/new-model"}}

	if err := SaveModels(want); err != nil {
		t.Fatalf("SaveModels() error = %v", err)
	}
	got, err := LoadModels()
	if err != nil {
		t.Fatalf("LoadModels() error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadModels() = %#v, want %#v", got, want)
	}

	info, err := os.Stat(filepath.Join(home, ".config", "gatekey-proxy", "models.json"))
	if err != nil {
		t.Fatalf("Stat(models.json) error = %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0600 {
		t.Fatalf("models.json mode = %o, want 600", gotMode)
	}
}

func TestValidateModelsRejectsDuplicates(t *testing.T) {
	err := ValidateModels([]ModelSetting{
		{Provider: "Groq", Model: "model-a"},
		{Provider: "groq", Model: "model-a"},
	})
	if err == nil {
		t.Fatal("ValidateModels() error = nil, want duplicate error")
	}
}
