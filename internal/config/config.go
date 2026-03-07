// Package config manages the opsagent configuration file (API keys, default provider).
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", appName), nil
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
	return filepath.Join(dir, "config.env"), nil
}

// Load reads the managed config file and sets environment variables from it.
// Existing environment variables always win.
func Load() error {
	path, err := FilePath()
	if err != nil {
		return err
	}
	return loadIfExists(path)
}

// Read returns all key=value pairs from the config file.
func Read() (map[string]string, error) {
	path, err := FilePath()
	if err != nil {
		return nil, err
	}
	return readEnvFile(path)
}

// Set writes a key=value pair to the config file (creates the file and directory if needed).
func Set(key, value string) error {
	path, err := FilePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	existing, _ := readEnvFile(path)
	if existing == nil {
		existing = make(map[string]string)
	}
	existing[key] = value
	return writeEnvFile(path, existing)
}

// Delete removes a key from the config file.
func Delete(key string) error {
	path, err := FilePath()
	if err != nil {
		return err
	}
	existing, _ := readEnvFile(path)
	if existing == nil {
		return nil
	}
	delete(existing, key)
	return writeEnvFile(path, existing)
}

func loadEnvFile(path string) error {
	pairs, err := readEnvFile(path)
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

func loadIfExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return loadEnvFile(path)
}

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer f.Close()
	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return result, scanner.Err()
}

func writeEnvFile(path string, pairs map[string]string) error {
	// Preserve comments and order from existing file, update/add keys.
	existingLines, _ := readFileLines(path)

	seen := make(map[string]bool)
	var lines []string
	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			lines = append(lines, line)
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			if val, ok := pairs[key]; ok {
				lines = append(lines, key+"="+val)
				seen[key] = true
				continue
			}
		}
		lines = append(lines, line)
	}
	for k, v := range pairs {
		if !seen[k] {
			lines = append(lines, k+"="+v)
		}
	}

	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0600)
}

func readFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
