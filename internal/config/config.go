// Package config manages the opsagent configuration file (API keys, default provider).
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const appName = "opsagent"

// Dir returns the OS-appropriate config directory for opsagent.
func Dir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, appName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "AppData", "Roaming", appName), nil
	default:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", appName), nil
	}
}

// FilePath returns the full path to the config file.
func FilePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Load reads the managed config file and sets environment variables from it.
// If a legacy config.env exists but config.yaml does not, it migrates automatically.
// Existing environment variables always win.
func Load() error {
	path, err := FilePath()
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
		migrateFromEnv(path)
	}

	pairs, err := readYAML(path)
	if err != nil {
		return err
	}
	for k, v := range pairs {
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
	return nil
}

// migrateFromEnv converts a legacy config.env to config.yaml.
func migrateFromEnv(yamlPath string) {
	envPath := strings.TrimSuffix(yamlPath, ".yaml") + ".env"
	f, err := os.Open(envPath)
	if err != nil {
		return
	}
	defer f.Close()

	pairs := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			pairs[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	if len(pairs) == 0 {
		return
	}

	os.MkdirAll(filepath.Dir(yamlPath), 0700)
	if err := writeYAML(yamlPath, pairs); err == nil {
		os.Remove(envPath)
	}
}

// Read returns all key-value pairs from the config file.
func Read() (map[string]string, error) {
	path, err := FilePath()
	if err != nil {
		return nil, err
	}
	return readYAML(path)
}

// Set writes a key-value pair to the config file (creates the file and directory if needed).
func Set(key, value string) error {
	path, err := FilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	existing, _ := readYAML(path)
	if existing == nil {
		existing = make(map[string]string)
	}
	existing[key] = value
	return writeYAML(path, existing)
}

// Delete removes a key from the config file.
func Delete(key string) error {
	path, err := FilePath()
	if err != nil {
		return err
	}
	existing, _ := readYAML(path)
	if existing == nil {
		return nil
	}
	delete(existing, key)
	return writeYAML(path, existing)
}

func readYAML(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return map[string]string{}, nil
	}
	result := make(map[string]string)
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse config.yaml: %w", err)
	}
	return result, nil
}

func writeYAML(path string, pairs map[string]string) error {
	data, err := yaml.Marshal(pairs)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return os.WriteFile(path, data, 0600)
}
