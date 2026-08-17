package desktop

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLaunchAtLoginToggle(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	if IsLaunchAtLoginEnabled() {
		t.Fatal("IsLaunchAtLoginEnabled() = true initially, want false")
	}

	if err := SetLaunchAtLogin(true); err != nil {
		t.Fatalf("SetLaunchAtLogin(true) error = %v", err)
	}

	if !IsLaunchAtLoginEnabled() {
		t.Fatal("IsLaunchAtLoginEnabled() = false after enable, want true")
	}

	plistPath := filepath.Join(tempHome, "Library", "LaunchAgents", launchAgentLabel+".plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatalf("ReadFile(plist) error = %v", err)
	}
	if len(data) == 0 {
		t.Fatal("plist file is empty")
	}

	if err := SetLaunchAtLogin(false); err != nil {
		t.Fatalf("SetLaunchAtLogin(false) error = %v", err)
	}

	if IsLaunchAtLoginEnabled() {
		t.Fatal("IsLaunchAtLoginEnabled() = true after disable, want false")
	}
}
