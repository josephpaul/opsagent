package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/josephpaul/opsagent/internal/config"
	"github.com/josephpaul/opsagent/internal/telegram"
	"github.com/spf13/cobra"
)

const loginNotifyCommand = "login notify"

var (
	loginPrintOnly bool
	loginPamFiles  []string
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Handle PAM-triggered login notifications",
	Long: `Login notification helpers for PAM-based session alerts.

Typical setup:
  1. Save Telegram settings:
     opsagent config set-key TELEGRAM_BOT_TOKEN <token>
     opsagent config set-key TELEGRAM_CHAT_ID <chat_id>
  2. (Optional) Enable/disable explicitly:
     opsagent config set-key LOGIN_NOTIFY_ENABLED true
  3. Print PAM setup instructions:
     opsagent login install-pam
  4. Verify configuration:
     opsagent login status

PAM should call:
  opsagent login notify

This command reads PAM_* environment variables and sends a Telegram alert.`,
}

var loginNotifyCmd = &cobra.Command{
	Use:    "notify",
	Short:  "Internal command called by PAM on successful session open",
	Hidden: true,
	RunE:   runLoginNotify,
}

var loginInstallPamCmd = &cobra.Command{
	Use:   "install-pam",
	Short: "Install the PAM hook for login notifications",
	RunE:  runLoginInstallPam,
}

var loginStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check login notification configuration and PAM integration hints",
	RunE:  runLoginStatus,
}

func init() {
	loginInstallPamCmd.Flags().BoolVar(&loginPrintOnly, "print-only", false, "Only print the PAM snippet instead of editing files")
	loginInstallPamCmd.Flags().StringSliceVar(&loginPamFiles, "file", nil, "PAM file(s) to update (defaults to /etc/pam.d/sshd and /etc/pam.d/login)")

	loginCmd.AddCommand(loginNotifyCmd)
	loginCmd.AddCommand(loginInstallPamCmd)
	loginCmd.AddCommand(loginStatusCmd)
	rootCmd.AddCommand(loginCmd)
}

func runLoginNotify(cmd *cobra.Command, args []string) error {
	enabled := strings.ToLower(strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_ENABLED")))
	if enabled == "false" || enabled == "0" || enabled == "no" {
		return nil
	}

	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	chatIDText := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	if token == "" || chatIDText == "" {
		return nil
	}

	chatID, err := strconv.ParseInt(chatIDText, 10, 64)
	if err != nil || chatID == 0 {
		return nil
	}

	message, err := buildLoginNotification()
	if err != nil {
		fmt.Fprintf(os.Stderr, "login notify: %v\n", err)
		return nil
	}

	if err := telegram.Notify(token, chatID, message); err != nil {
		fmt.Fprintf(os.Stderr, "login notify telegram error: %v\n", err)
	}
	return nil
}

func runLoginInstallPam(cmd *cobra.Command, args []string) error {
	snippet, configPath, err := pamSnippet()
	if err != nil {
		return err
	}

	fmt.Println("Recommended PAM snippet:")
	fmt.Println()
	fmt.Printf("  %s\n", snippet)
	fmt.Println()

	targets := selectedPamTargets()
	if loginPrintOnly {
		fmt.Println("Suggested PAM files to update:")
		for _, path := range targets {
			if fileExists(path) {
				fmt.Printf("  - %s\n", path)
			}
		}
		fmt.Println()
		fmt.Printf("Config path used by PAM: %s\n", configPath)
		fmt.Println()
		fmt.Println("Notes:")
		fmt.Println("  - Use 'optional' so logins are never blocked if notification fails.")
		fmt.Println("  - Keep an active root/admin session open while testing PAM changes.")
		fmt.Println("  - 'opsagent login notify' reads TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID from opsagent config.")
		fmt.Println("  - You may optionally set LOGIN_NOTIFY_ENABLED=false to disable notifications without removing the PAM line.")
		return nil
	}

	if len(targets) == 0 {
		return fmt.Errorf("no PAM target files found")
	}

	var updated []string
	for _, path := range targets {
		changed, err := installPamHook(path, snippet)
		if err != nil {
			return err
		}
		if changed {
			updated = append(updated, path)
		}
	}

	if len(updated) > 0 {
		fmt.Println("Updated PAM files:")
		for _, path := range updated {
			fmt.Printf("  - %s\n", path)
			fmt.Printf("    Backup: %s\n", path+".opsagent.bak")
		}
		fmt.Println()
	} else {
		fmt.Println("PAM hook already present in target files.")
		fmt.Println()
	}

	fmt.Printf("Config path used by PAM: %s\n", configPath)
	fmt.Println()
	fmt.Println("Notes:")
	fmt.Println("  - Use 'optional' so logins are never blocked if notification fails.")
	fmt.Println("  - Keep an active root/admin session open while testing PAM changes.")
	fmt.Println("  - 'opsagent login notify' reads TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID from opsagent config.")
	fmt.Println("  - You may optionally set LOGIN_NOTIFY_ENABLED=false to disable notifications without removing the PAM line.")
	return nil
}

func runLoginStatus(cmd *cobra.Command, args []string) error {
	token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	chatID := strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID"))
	enabled := strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_ENABLED"))
	if enabled == "" {
		enabled = "true (default)"
	}
	hostLabel := strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_HOSTNAME_LABEL"))
	if hostLabel == "" {
		hostLabel, _ = os.Hostname()
		if hostLabel == "" {
			hostLabel = "(auto hostname unavailable)"
		}
	}
	timezone := strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_TIMEZONE"))
	if timezone == "" {
		timezone = "UTC (default)"
	}
	timeFormat := strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_TIME_FORMAT"))
	if timeFormat == "" {
		timeFormat = loginDefaultTimeFormat + " (default)"
	}

	fmt.Println("Login Notifications")
	if token != "" {
		fmt.Printf("  Token:        %s\n", maskValue("TELEGRAM_BOT_TOKEN", token))
	} else {
		fmt.Println("  Token:        missing")
	}
	if chatID != "" {
		fmt.Printf("  Chat ID:      %s\n", chatID)
	} else {
		fmt.Println("  Chat ID:      missing")
	}
	fmt.Printf("  Enabled:      %s\n", enabled)
	fmt.Printf("  Host label:   %s\n", hostLabel)
	fmt.Printf("  Time zone:    %s\n", timezone)
	fmt.Printf("  Time format:  %s\n", timeFormat)

	pamExecPath := detectPamExec()
	if pamExecPath != "" {
		fmt.Printf("  pam_exec.so:  %s\n", pamExecPath)
	} else {
		fmt.Println("  pam_exec.so:  not found in common paths")
	}

	fmt.Println()
	fmt.Println("PAM Targets")
	for _, path := range pamTargetFiles() {
		status := "missing"
		if fileExists(path) {
			status = "present"
		}
		integrated := ""
		if ok, _ := pamFileContainsOpsagent(path); ok {
			integrated = " (opsagent hook found)"
		}
		fmt.Printf("  %s: %s%s\n", path, status, integrated)
	}

	fmt.Println()
	fmt.Println("Expected command:")
	snippet, configPath, err := pamSnippet()
	if err != nil {
		return err
	}
	fmt.Printf("  %s\n", snippet)
	fmt.Printf("  Config path: %s\n", configPath)
	return nil
}

func buildLoginNotification() (string, error) {
	user := firstNonEmpty(os.Getenv("PAM_USER"), os.Getenv("USER"))
	if user == "" {
		return "", fmt.Errorf("PAM_USER not available")
	}

	host := strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_HOSTNAME_LABEL"))
	if host == "" {
		host, _ = os.Hostname()
	}
	if host == "" {
		host = "unknown"
	}

	source := firstNonEmpty(os.Getenv("PAM_RHOST"), "local")
	service := firstNonEmpty(os.Getenv("PAM_SERVICE"), "unknown")
	tty := firstNonEmpty(os.Getenv("PAM_TTY"), "unknown")
	event := loginEventFromPAMType(os.Getenv("PAM_TYPE"))
	location, format, err := loginTimeSettings()
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"Session activity detected\nEvent: %s\nUser: %s\nSource: %s\nService: %s\nTTY: %s\nHost: %s\nTime: %s",
		event,
		user,
		source,
		service,
		tty,
		host,
		time.Now().In(location).Format(format),
	), nil
}

func loginEventFromPAMType(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "open_session":
		return "Login"
	case "close_session":
		return "Logout"
	default:
		return "Session"
	}
}

const loginDefaultTimeFormat = "2006-01-02 15:04:05 MST"

func loginTimeSettings() (*time.Location, string, error) {
	tz := strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_TIMEZONE"))
	location := time.UTC
	if tz != "" {
		loaded, err := time.LoadLocation(tz)
		if err != nil {
			return nil, "", fmt.Errorf("invalid LOGIN_NOTIFY_TIMEZONE %q: %w", tz, err)
		}
		location = loaded
	}

	format := strings.TrimSpace(os.Getenv("LOGIN_NOTIFY_TIME_FORMAT"))
	if format == "" {
		format = loginDefaultTimeFormat
	}
	return location, format, nil
}

func pamTargetFiles() []string {
	return []string{
		"/etc/pam.d/sshd",
		"/etc/pam.d/login",
	}
}

func selectedPamTargets() []string {
	targets := loginPamFiles
	if len(targets) == 0 {
		targets = pamTargetFiles()
	}
	var out []string
	for _, path := range targets {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if fileExists(path) {
			out = append(out, path)
		}
	}
	return out
}

func pamSnippet() (string, string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("resolve current binary: %w", err)
	}
	execPath, _ = filepath.EvalSymlinks(execPath)

	configPath, err := loginConfigPath()
	if err != nil {
		return "", "", err
	}
	snippet := fmt.Sprintf("session optional pam_exec.so /usr/bin/env OPSAGENT_CONFIG_PATH=%s %s %s", configPath, execPath, loginNotifyCommand)
	return snippet, configPath, nil
}

func loginConfigPath() (string, error) {
	if sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER")); sudoUser != "" && sudoUser != "root" {
		u, err := user.Lookup(sudoUser)
		if err == nil && strings.TrimSpace(u.HomeDir) != "" {
			return filepath.Join(u.HomeDir, ".config", "opsagent", "config.yaml"), nil
		}
	}
	return config.FilePath()
}

func installPamHook(path, snippet string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	lines := strings.Split(string(data), "\n")
	found := false
	changed := false
	for i, line := range lines {
		if strings.Contains(line, "pam_exec.so") && strings.Contains(line, loginNotifyCommand) {
			found = true
			if strings.TrimSpace(line) == snippet {
				return false, nil
			}
			lines[i] = snippet
			changed = true
		}
	}

	if !found {
		var buf bytes.Buffer
		buf.Write(data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteString("\n# OpsAgent login notification\n")
		buf.WriteString(snippet)
		buf.WriteByte('\n')
		lines = strings.Split(buf.String(), "\n")
		changed = true
	}

	if !changed {
		return false, nil
	}

	backupPath := path + ".opsagent.bak"
	if !fileExists(backupPath) {
		if err := os.WriteFile(backupPath, data, 0600); err != nil {
			return false, fmt.Errorf("write backup %s: %w", backupPath, err)
		}
	}

	newData := strings.Join(lines, "\n")
	if err := os.WriteFile(path, []byte(newData), 0644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

func detectPamExec() string {
	candidates := []string{
		"/lib/security/pam_exec.so",
		"/lib64/security/pam_exec.so",
		"/usr/lib/security/pam_exec.so",
		"/usr/lib64/security/pam_exec.so",
		"/usr/lib/x86_64-linux-gnu/security/pam_exec.so",
		"/usr/lib/aarch64-linux-gnu/security/pam_exec.so",
	}
	for _, path := range candidates {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func pamFileContainsOpsagent(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := string(data)
	return strings.Contains(text, "pam_exec.so") && strings.Contains(text, loginNotifyCommand), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
