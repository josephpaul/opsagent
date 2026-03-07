package monitor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
)

const serviceName = "opsagent-monitor"

// ServiceConfig holds the parameters for generating service files.
type ServiceConfig struct {
	BinaryPath   string
	Interval     string
	CPUThreshold int
	RAMThreshold int
	LogPath      string
}

// Install generates and installs the OS-appropriate background service.
func Install(cfg ServiceConfig) error {
	switch runtime.GOOS {
	case "linux":
		return installSystemd(cfg)
	case "darwin":
		return installLaunchd(cfg)
	default:
		return fmt.Errorf("automatic service install is not supported on %s; use 'opsagent monitor run' manually", runtime.GOOS)
	}
}

// Uninstall removes the background service.
func Uninstall() error {
	switch runtime.GOOS {
	case "linux":
		return uninstallSystemd()
	case "darwin":
		return uninstallLaunchd()
	default:
		return fmt.Errorf("automatic service uninstall is not supported on %s", runtime.GOOS)
	}
}

// Start enables and starts the service.
func Start() error {
	switch runtime.GOOS {
	case "linux":
		return run("systemctl", "--user", "start", serviceName+".service")
	case "darwin":
		path := launchdPlistPath()
		return run("launchctl", "load", path)
	default:
		return fmt.Errorf("not supported on %s", runtime.GOOS)
	}
}

// Stop stops the service.
func Stop() error {
	switch runtime.GOOS {
	case "linux":
		return run("systemctl", "--user", "stop", serviceName+".service")
	case "darwin":
		path := launchdPlistPath()
		return run("launchctl", "unload", path)
	default:
		return fmt.Errorf("not supported on %s", runtime.GOOS)
	}
}

// StatusInfo holds the current state and configuration of the monitor service.
type StatusInfo struct {
	Installed    bool
	Running      bool
	ServiceFile  string
	Interval     string
	CPUThreshold string
	RAMThreshold string
	LogPath      string
	BinaryPath   string
}

// Status returns the current state and configuration of the monitor service.
func Status() (*StatusInfo, error) {
	switch runtime.GOOS {
	case "linux":
		return statusSystemd()
	case "darwin":
		return statusLaunchd()
	default:
		return nil, fmt.Errorf("not supported on %s", runtime.GOOS)
	}
}

func statusSystemd() (*StatusInfo, error) {
	path := filepath.Join(systemdDir(), serviceName+".service")
	info := &StatusInfo{ServiceFile: path}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return info, nil
	}
	info.Installed = true
	parseServiceFile(info, path)

	out, err := exec.Command("systemctl", "--user", "is-active", serviceName+".service").CombinedOutput()
	if err == nil && strings.TrimSpace(string(out)) == "active" {
		info.Running = true
	}
	return info, nil
}

func statusLaunchd() (*StatusInfo, error) {
	path := launchdPlistPath()
	info := &StatusInfo{ServiceFile: path}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return info, nil
	}
	info.Installed = true
	parsePlistFile(info, path)

	out, _ := exec.Command("launchctl", "list").CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "com.opsagent.monitor") {
			fields := strings.Fields(line)
			if len(fields) >= 1 && fields[0] != "-" {
				info.Running = true
			}
			break
		}
	}
	return info, nil
}

func parseServiceFile(info *StatusInfo, path string) {
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
				case "--interval":
					if i+1 < len(args) {
						info.Interval = args[i+1]
					}
				case "--cpu-threshold":
					if i+1 < len(args) {
						info.CPUThreshold = args[i+1]
					}
				case "--ram-threshold":
					if i+1 < len(args) {
						info.RAMThreshold = args[i+1]
					}
				case "--log":
					if i+1 < len(args) {
						info.LogPath = args[i+1]
					}
				}
			}
		}
	}
}

func parsePlistFile(info *StatusInfo, path string) {
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
		case "--interval":
			if i+1 < len(args) {
				info.Interval = args[i+1]
			}
		case "--cpu-threshold":
			if i+1 < len(args) {
				info.CPUThreshold = args[i+1]
			}
		case "--ram-threshold":
			if i+1 < len(args) {
				info.RAMThreshold = args[i+1]
			}
		case "--log":
			if i+1 < len(args) {
				info.LogPath = args[i+1]
			}
		}
	}
}

// --- systemd (Linux) ---

const systemdServiceTmpl = `[Unit]
Description=OpsAgent Background Monitor
After=network.target

[Service]
Type=simple
ExecStart={{.BinaryPath}} monitor run --interval {{.Interval}} --cpu-threshold {{.CPUThreshold}} --ram-threshold {{.RAMThreshold}}{{if .LogPath}} --log {{.LogPath}}{{end}}
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
`

func systemdDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user")
}

func installSystemd(cfg ServiceConfig) error {
	dir := systemdDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, serviceName+".service")

	tmpl, err := template.New("service").Parse(systemdServiceTmpl)
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

	if err := run("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := run("systemctl", "--user", "enable", serviceName+".service"); err != nil {
		return err
	}
	if err := run("systemctl", "--user", "start", serviceName+".service"); err != nil {
		return err
	}
	fmt.Printf("Installed and started: %s\n", path)
	fmt.Printf("  Logs: journalctl --user -u %s -f\n", serviceName)
	return nil
}

func uninstallSystemd() error {
	_ = run("systemctl", "--user", "stop", serviceName+".service")
	_ = run("systemctl", "--user", "disable", serviceName+".service")
	path := filepath.Join(systemdDir(), serviceName+".service")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = run("systemctl", "--user", "daemon-reload")
	fmt.Printf("Removed: %s\n", path)
	return nil
}

// --- launchd (macOS) ---

const launchdPlistTmpl = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.opsagent.monitor</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>monitor</string>
        <string>run</string>
        <string>--interval</string>
        <string>{{.Interval}}</string>
        <string>--cpu-threshold</string>
        <string>{{.CPUThresholdStr}}</string>
        <string>--ram-threshold</string>
        <string>{{.RAMThresholdStr}}</string>{{if .LogPath}}
        <string>--log</string>
        <string>{{.LogPath}}</string>{{end}}
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

type launchdData struct {
	ServiceConfig
	CPUThresholdStr string
	RAMThresholdStr string
	StdoutLog       string
	StderrLog       string
}

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.opsagent.monitor.plist")
}

func installLaunchd(cfg ServiceConfig) error {
	path := launchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, ".config", "opsagent", "logs")
	os.MkdirAll(logDir, 0755)

	data := launchdData{
		ServiceConfig:   cfg,
		CPUThresholdStr: fmt.Sprintf("%d", cfg.CPUThreshold),
		RAMThresholdStr: fmt.Sprintf("%d", cfg.RAMThreshold),
		StdoutLog:       filepath.Join(logDir, "monitor.stdout.log"),
		StderrLog:       filepath.Join(logDir, "monitor.stderr.log"),
	}

	tmpl, err := template.New("plist").Parse(launchdPlistTmpl)
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

	if err := run("launchctl", "load", path); err != nil {
		return err
	}
	fmt.Printf("Installed and started: %s\n", path)
	fmt.Printf("  Stdout log: %s\n", data.StdoutLog)
	fmt.Printf("  Stderr log: %s\n", data.StderrLog)
	return nil
}

func uninstallLaunchd() error {
	path := launchdPlistPath()
	_ = run("launchctl", "unload", path)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("Removed: %s\n", path)
	return nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
