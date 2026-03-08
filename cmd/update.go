package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/josephpaul/opsagent/internal/monitor"
	"github.com/josephpaul/opsagent/internal/telegram"
	"github.com/spf13/cobra"
)

const (
	githubRepo = "josephpaul/opsagent"
	githubAPI  = "https://api.github.com/repos/" + githubRepo + "/releases/latest"
)

var checkOnly bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update opsagent to the latest version",
	Long: `Check for and install the latest version of opsagent from GitHub.

  opsagent update          Download and install the latest release
  opsagent update --check  Check for updates without installing`,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&checkOnly, "check", false, "Only check for updates, don't install")
	rootCmd.AddCommand(updateCmd)
}

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Name    string        `json:"name"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
	HTMLURL string        `json:"html_url"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func runUpdate(cmd *cobra.Command, args []string) error {
	fmt.Println("Checking for updates...")

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", githubAPI, nil)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "opsagent/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Println("No releases found. You may be running a development build.")
		fmt.Printf("Current version: %s\n", Version)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("parse release info: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(Version, "v")

	if latestVersion == currentVersion {
		fmt.Printf("You're already on the latest version (%s).\n", Version)
		return nil
	}

	fmt.Printf("Current version:  %s\n", Version)
	fmt.Printf("Latest version:   %s\n", latestVersion)
	if release.HTMLURL != "" {
		fmt.Printf("Release notes:    %s\n", release.HTMLURL)
	}

	if checkOnly {
		fmt.Println("\nRun 'opsagent update' to install the update.")
		return nil
	}

	assetName := buildAssetName()
	var downloadURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL != "" {
		return updateFromBinary(client, downloadURL)
	}

	fmt.Println("\nNo pre-built binary found for your platform. Building from source...")
	return updateFromSource()
}

func buildAssetName() string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	ext := ""
	if os == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("opsagent-%s-%s%s", os, arch, ext)
}

func updateFromBinary(client *http.Client, downloadURL string) error {
	fmt.Printf("Downloading %s...\n", downloadURL)

	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current binary: %w", err)
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(currentBinary), "opsagent-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write binary: %w", err)
	}
	tmpFile.Close()

	if err := os.Chmod(tmpPath, 0755); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(tmpPath, currentBinary); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("replace binary: %w (you may need sudo)", err)
	}

	fmt.Println("Update complete!")
	restartServices()
	return nil
}

func updateFromSource() error {
	if _, err := exec.LookPath("go"); err != nil {
		return fmt.Errorf("Go is not installed — cannot build from source. Install Go from https://go.dev/dl/ or download a pre-built release from https://github.com/%s/releases", githubRepo)
	}

	tmpDir, err := os.MkdirTemp("", "opsagent-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	fmt.Println("Cloning repository...")
	clone := exec.Command("git", "clone", "--depth=1", "https://github.com/"+githubRepo+".git", tmpDir)
	clone.Stdout = os.Stdout
	clone.Stderr = os.Stderr
	if err := clone.Run(); err != nil {
		return fmt.Errorf("git clone: %w", err)
	}

	fmt.Println("Building...")
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find current binary: %w", err)
	}
	currentBinary, err = filepath.EvalSymlinks(currentBinary)
	if err != nil {
		return fmt.Errorf("resolve binary path: %w", err)
	}

	outputName := "opsagent"
	if runtime.GOOS == "windows" {
		outputName = "opsagent.exe"
	}
	outputPath := filepath.Join(tmpDir, outputName)

	build := exec.Command("go", "build", "-o", outputPath, ".")
	build.Dir = tmpDir
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}

	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	if err := os.Rename(outputPath, currentBinary); err != nil {
		return fmt.Errorf("replace binary: %w (you may need sudo)", err)
	}

	fmt.Println("Update complete! Built from source.")
	restartServices()
	return nil
}

func restartServices() {
	fmt.Println()
	fmt.Println("Checking running services...")

	tgRestarted := restartTelegram()
	monRestarted := restartMonitor()

	if !tgRestarted && !monRestarted {
		fmt.Println("  No services to restart.")
	}
}

func restartTelegram() bool {
	info, err := telegram.ServiceStatus()
	if err != nil || !info.Installed {
		return false
	}

	if !info.Running {
		fmt.Printf("  Telegram bot: installed but was not running (skipped)\n")
		return false
	}

	fmt.Printf("  Telegram bot: restarting...")
	if err := telegram.StopService(); err != nil {
		fmt.Printf(" stop failed: %v\n", err)
		return false
	}

	time.Sleep(1 * time.Second)

	if err := telegram.StartService(); err != nil {
		fmt.Printf(" start failed: %v\n", err)
		return false
	}

	fmt.Printf(" %sdone%s\n", colorGreen, colorReset)
	return true
}

func restartMonitor() bool {
	info, err := monitor.Status()
	if err != nil || !info.Installed {
		return false
	}

	if !info.Running {
		fmt.Printf("  Monitor: installed but was not running (skipped)\n")
		return false
	}

	fmt.Printf("  Monitor: restarting...")
	if err := monitor.Stop(); err != nil {
		fmt.Printf(" stop failed: %v\n", err)
		return false
	}

	time.Sleep(1 * time.Second)

	if err := monitor.Start(); err != nil {
		fmt.Printf(" start failed: %v\n", err)
		return false
	}

	fmt.Printf(" %sdone%s\n", colorGreen, colorReset)
	return true
}
