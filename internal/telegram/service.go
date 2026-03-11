package telegram

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

const tgServiceName = "opsagent-telegram"

// ServiceConfig holds the parameters for generating the Telegram bot service.
type ServiceConfig struct {
	BinaryPath    string
	Mode          string // "webhook" or "poll"
	WebhookURL    string
	Port          int
	WebhookSecret string
}

// ServiceStatusInfo holds the current state of the Telegram bot service.
type ServiceStatusInfo struct {
	Installed   bool
	Running     bool
	ServiceFile string
	BinaryPath  string
	Mode        string
	WebhookURL  string
	Port        string
}

// InstallService generates and installs the OS-appropriate background service.
func InstallService(cfg ServiceConfig) error {
	switch runtime.GOOS {
	case "linux":
		return installTGSystemd(cfg)
	case "darwin":
		return installTGLaunchd(cfg)
	default:
		return fmt.Errorf("automatic service install is not supported on %s; run 'opsagent telegram %s' manually or use nohup", runtime.GOOS, cfg.Mode)
	}
}

// UninstallService removes the Telegram bot service.
func UninstallService() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallTGSystemd()
	case "darwin":
		return uninstallTGLaunchd()
	default:
		return fmt.Errorf("automatic service uninstall is not supported on %s", runtime.GOOS)
	}
}

// StartService starts the installed service.
func StartService() error {
	switch runtime.GOOS {
	case "linux":
		return runCmd("systemctl", "--user", "start", tgServiceName+".service")
	case "darwin":
		return runCmd("launchctl", "load", tgLaunchdPlistPath())
	default:
		return fmt.Errorf("not supported on %s", runtime.GOOS)
	}
}

// StopService stops the service.
func StopService() error {
	switch runtime.GOOS {
	case "linux":
		return runCmd("systemctl", "--user", "stop", tgServiceName+".service")
	case "darwin":
		return runCmd("launchctl", "unload", tgLaunchdPlistPath())
	default:
		return fmt.Errorf("not supported on %s", runtime.GOOS)
	}
}

// ServiceStatus returns the current state of the Telegram bot service.
func ServiceStatus() (*ServiceStatusInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return statusTGSystemd()
	case "darwin":
		return statusTGLaunchd()
	default:
		return nil, fmt.Errorf("not supported on %s", runtime.GOOS)
	}
}

// --- systemd (Linux) ---

const tgSystemdTmpl = `[Unit]
Description=OpsAgent Telegram Bot
After=network.target

[Service]
Type=simple
ExecStart={{.BinaryPath}} telegram {{.Mode}}{{if eq .Mode "webhook"}} --webhook-url {{.WebhookURL}} --port {{.Port}}{{if .WebhookSecret}} --webhook-secret {{.WebhookSecret}}{{end}}{{end}}
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`

func tgSystemdDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func installTGSystemd(cfg ServiceConfig) error {
	dir := tgSystemdDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, tgServiceName+".service")

	tmpl, err := template.New("service").Parse(tgSystemdTmpl)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tmpl.Execute(f, cfg); err != nil {
		return err
	}

	if err := runCmd("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := runCmd("systemctl", "--user", "enable", tgServiceName+".service"); err != nil {
		return err
	}
	if err := runCmd("systemctl", "--user", "start", tgServiceName+".service"); err != nil {
		return err
	}

	enableLinger()

	fmt.Printf("Installed and started: %s\n", path)
	fmt.Printf("  Logs: journalctl --user -u %s -f\n", tgServiceName)
	return nil
}

func uninstallTGSystemd() error {
	_ = runCmd("systemctl", "--user", "stop", tgServiceName+".service")
	_ = runCmd("systemctl", "--user", "disable", tgServiceName+".service")
	path := filepath.Join(tgSystemdDir(), tgServiceName+".service")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = runCmd("systemctl", "--user", "daemon-reload")
	fmt.Printf("Removed: %s\n", path)
	return nil
}

func statusTGSystemd() (*ServiceStatusInfo, error) {
	path := filepath.Join(tgSystemdDir(), tgServiceName+".service")
	info := &ServiceStatusInfo{ServiceFile: path}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return info, nil
	}
	info.Installed = true
	parseTGServiceFile(info, path)

	out, err := exec.Command("systemctl", "--user", "is-active", tgServiceName+".service").CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		info.Running = true
	}
	return info, nil
}

func parseTGServiceFile(info *ServiceStatusInfo, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ExecStart=") {
			args := strings.Fields(strings.TrimPrefix(line, "ExecStart="))
			if len(args) > 0 {
				info.BinaryPath = args[0]
			}
			for i, a := range args {
				switch a {
				case "webhook", "poll":
					info.Mode = a
				case "--webhook-url":
					if i+1 < len(args) {
						info.WebhookURL = args[i+1]
					}
				case "--port":
					if i+1 < len(args) {
						info.Port = args[i+1]
					}
				}
			}
		}
	}
}

// --- launchd (macOS) ---

const tgLaunchdPlistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.opsagent.telegram</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>telegram</string>
        <string>{{.Mode}}</string>{{if eq .Mode "webhook"}}
        <string>--webhook-url</string>
        <string>{{.WebhookURL}}</string>
        <string>--port</string>
        <string>{{.PortStr}}</string>{{if .WebhookSecret}}
        <string>--webhook-secret</string>
        <string>{{.WebhookSecret}}</string>{{end}}{{end}}
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.StdoutLog}}</string>
    <key>StandardErrorPath</key>
    <string>{{.StderrLog}}</string>
</dict>
</plist>
`

type tgLaunchdData struct {
	ServiceConfig
	PortStr   string
	StdoutLog string
	StderrLog string
}

func tgLaunchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.opsagent.telegram.plist")
}

func installTGLaunchd(cfg ServiceConfig) error {
	path := tgLaunchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".config", "opsagent", "logs")
	os.MkdirAll(logDir, 0755)

	data := tgLaunchdData{
		ServiceConfig: cfg,
		PortStr:       fmt.Sprintf("%d", cfg.Port),
		StdoutLog:     filepath.Join(logDir, "telegram.stdout.log"),
		StderrLog:     filepath.Join(logDir, "telegram.stderr.log"),
	}

	tmpl, err := template.New("plist").Parse(tgLaunchdPlistTmpl)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := tmpl.Execute(f, data); err != nil {
		return err
	}

	if err := runCmd("launchctl", "load", path); err != nil {
		return err
	}
	fmt.Printf("Installed and started: %s\n", path)
	fmt.Printf("  Stdout log: %s\n", data.StdoutLog)
	fmt.Printf("  Stderr log: %s\n", data.StderrLog)
	return nil
}

func uninstallTGLaunchd() error {
	path := tgLaunchdPlistPath()
	_ = runCmd("launchctl", "unload", path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("Removed: %s\n", path)
	return nil
}

func statusTGLaunchd() (*ServiceStatusInfo, error) {
	path := tgLaunchdPlistPath()
	info := &ServiceStatusInfo{ServiceFile: path}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return info, nil
	}
	info.Installed = true
	parseTGPlistFile(info, path)

	out, _ := exec.Command("launchctl", "list").CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "com.opsagent.telegram") {
			fields := strings.Fields(line)
			if len(fields) >= 1 && fields[0] != "-" {
				info.Running = true
			}
			break
		}
	}
	return info, nil
}

func parseTGPlistFile(info *ServiceStatusInfo, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	var args []string
	inArray := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "<key>ProgramArguments</key>" {
			inArray = true
			continue
		}
		if inArray && trimmed == "<array>" {
			continue
		}
		if inArray && trimmed == "</array>" {
			break
		}
		if inArray && strings.HasPrefix(trimmed, "<string>") {
			val := strings.TrimPrefix(trimmed, "<string>")
			val = strings.TrimSuffix(val, "</string>")
			args = append(args, val)
		}
	}
	if len(args) > 0 {
		info.BinaryPath = args[0]
	}
	for i, a := range args {
		switch a {
		case "webhook", "poll":
			info.Mode = a
		case "--webhook-url":
			if i+1 < len(args) {
				info.WebhookURL = args[i+1]
			}
		case "--port":
			if i+1 < len(args) {
				info.Port = args[i+1]
			}
		}
	}
}

func enableLinger() {
	user := os.Getenv("USER")
	if user == "" {
		return
	}
	if err := exec.Command("loginctl", "enable-linger", user).Run(); err == nil {
		fmt.Printf("  Enabled loginctl linger for user %s (service persists after logout)\n", user)
	}
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
