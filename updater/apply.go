package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ApplyStaged replaces the current executable with a previously verified
// staged release. The caller must relaunch after a successful replacement.
func ApplyStaged(current string) (bool, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("failed to locate the Gatekey executable: %w", err)
	}
	return applyStagedAt(current, executablePath)
}

func applyStagedAt(current, executablePath string) (bool, error) {
	manifest, err := loadStagedManifest()
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if _, err := parseVersion(current); err != nil {
		return false, nil
	}
	if compareVersions(manifest.Version, current) <= 0 {
		return false, nil
	}

	dir, err := updateDir()
	if err != nil {
		return false, err
	}
	stagedPath := filepath.Join(dir, "gatekey-proxy")
	if info, err := os.Lstat(executablePath); err != nil {
		return false, fmt.Errorf("failed to inspect the Gatekey executable: %w", err)
	} else if info.Mode()&os.ModeSymlink != 0 {
		return false, errors.New("Gatekey is installed through a symlink; update it with its package manager")
	}

	if err := verifyFileSHA256(stagedPath, manifest.BinarySHA256); err != nil {
		return false, err
	}
	if err := replaceExecutable(executablePath, stagedPath); err != nil {
		return false, err
	}
	os.Remove(stagedPath)
	if path, err := manifestPath(); err == nil {
		os.Remove(path)
	}
	return true, nil
}

func verifyFileSHA256(path, want string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open staged update: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, maxBinarySize+1)); err != nil {
		return fmt.Errorf("failed to verify staged update: %w", err)
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return errors.New("staged update checksum no longer matches")
	}
	return nil
}

func replaceExecutable(executablePath, stagedPath string) error {
	source, err := os.Open(stagedPath)
	if err != nil {
		return fmt.Errorf("failed to open staged update: %w", err)
	}
	defer source.Close()

	temp, err := os.CreateTemp(filepath.Dir(executablePath), ".gatekey-proxy-update-*")
	if err != nil {
		return fmt.Errorf("Gatekey cannot write to its install directory: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0755); err != nil {
		temp.Close()
		return err
	}
	if _, err := io.Copy(temp, io.LimitReader(source, maxBinarySize+1)); err != nil {
		temp.Close()
		return fmt.Errorf("failed to prepare the update: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("failed to sync the update: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("failed to close the update: %w", err)
	}
	if err := os.Rename(tempPath, executablePath); err != nil {
		return fmt.Errorf("failed to replace the Gatekey executable: %w", err)
	}
	return nil
}
