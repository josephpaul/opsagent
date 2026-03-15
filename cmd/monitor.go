package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/josephpaul/opsagent/agent"
	"github.com/josephpaul/opsagent/internal/config"
	"github.com/josephpaul/opsagent/internal/monitor"
	"github.com/spf13/cobra"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

var (
	monitorInterval     string
	monitorCPUThreshold int
	monitorRAMThreshold int
	monitorLogPath      string
	monitorTGToken      string
	monitorTGChatID     int64

	monitorSetInterval string
	monitorSetCPU      int
	monitorSetRAM      int
	monitorSetLog      string
	monitorSetClearLog bool
)

const (
	monitorIntervalKey = "MONITOR_INTERVAL"
	monitorCPUKey      = "MONITOR_CPU_THRESHOLD"
	monitorRAMKey      = "MONITOR_RAM_THRESHOLD"
	monitorLogKey      = "MONITOR_LOG_PATH"
)

type monitorSettings struct {
	Interval     string
	CPUThreshold int
	RAMThreshold int
	LogPath      string
}

type monitorConfigured struct {
	monitorSettings
	HasInterval bool
	HasCPU      bool
	HasRAM      bool
	HasLog      bool
}

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Background monitoring with automatic AI diagnosis on threshold breaches",
	Long: `Run a background monitor that polls CPU and RAM usage at a regular interval.
When usage exceeds the configured thresholds, the AI agent is automatically
invoked to diagnose the cause.

  opsagent monitor run                  Run in foreground (Ctrl+C to stop)
  opsagent monitor set                  Save default monitor settings in config.yaml
  opsagent monitor install              Install as a background service (systemd/launchd)
  opsagent monitor uninstall            Remove the background service
  opsagent monitor start                Start the installed service
  opsagent monitor stop                 Stop the installed service
  opsagent monitor status               Check if the service is running`,
}

var monitorRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the monitor in the foreground",
	RunE:  runMonitor,
}

var monitorInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install as a background service (systemd on Linux, launchd on macOS)",
	RunE:  runMonitorInstall,
}

var monitorSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Save default monitor settings in config.yaml",
	RunE:  runMonitorSet,
}

var monitorUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the background service",
	RunE:  runMonitorUninstall,
}

var monitorStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the installed background service",
	RunE:  runMonitorStart,
}

var monitorStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background service",
	RunE:  runMonitorStop,
}

var monitorStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the background service is running",
	RunE:  runMonitorStatus,
}

func init() {
	monitorRunCmd.Flags().StringVar(&monitorInterval, "interval", "60s", "Polling interval (e.g. 30s, 5m, 1h)")
	monitorRunCmd.Flags().IntVar(&monitorCPUThreshold, "cpu-threshold", 90, "CPU usage percentage to trigger alert")
	monitorRunCmd.Flags().IntVar(&monitorRAMThreshold, "ram-threshold", 85, "RAM usage percentage to trigger alert")
	monitorRunCmd.Flags().StringVar(&monitorLogPath, "log", "", "Log file path (also prints to stdout)")
	monitorRunCmd.Flags().StringVar(&monitorTGToken, "telegram-token", "", "Telegram bot token for alert notifications")
	monitorRunCmd.Flags().Int64Var(&monitorTGChatID, "telegram-chat-id", 0, "Telegram chat ID for alert notifications")

	monitorInstallCmd.Flags().StringVar(&monitorInterval, "interval", "60s", "Polling interval")
	monitorInstallCmd.Flags().IntVar(&monitorCPUThreshold, "cpu-threshold", 90, "CPU threshold (%)")
	monitorInstallCmd.Flags().IntVar(&monitorRAMThreshold, "ram-threshold", 85, "RAM threshold (%)")
	monitorInstallCmd.Flags().StringVar(&monitorLogPath, "log", "", "Log file path")
	monitorInstallCmd.Flags().StringVar(&monitorTGToken, "telegram-token", "", "Telegram bot token for alert notifications")
	monitorInstallCmd.Flags().Int64Var(&monitorTGChatID, "telegram-chat-id", 0, "Telegram chat ID for alert notifications")

	monitorSetCmd.Flags().StringVar(&monitorSetInterval, "interval", "", "Save default polling interval (e.g. 30s, 5m, 1h)")
	monitorSetCmd.Flags().IntVar(&monitorSetCPU, "cpu-threshold", 0, "Save default CPU threshold (%)")
	monitorSetCmd.Flags().IntVar(&monitorSetRAM, "ram-threshold", 0, "Save default RAM threshold (%)")
	monitorSetCmd.Flags().StringVar(&monitorSetLog, "log", "", "Save default log file path")
	monitorSetCmd.Flags().BoolVar(&monitorSetClearLog, "clear-log", false, "Remove MONITOR_LOG_PATH from config")

	monitorCmd.AddCommand(monitorRunCmd)
	monitorCmd.AddCommand(monitorSetCmd)
	monitorCmd.AddCommand(monitorInstallCmd)
	monitorCmd.AddCommand(monitorUninstallCmd)
	monitorCmd.AddCommand(monitorStartCmd)
	monitorCmd.AddCommand(monitorStopCmd)
	monitorCmd.AddCommand(monitorStatusCmd)
	rootCmd.AddCommand(monitorCmd)
}

func runMonitor(cmd *cobra.Command, args []string) error {
	settings, err := resolveMonitorSettings(cmd)
	if err != nil {
		return err
	}

	interval, err := time.ParseDuration(settings.Interval)
	if err != nil {
		return fmt.Errorf("invalid interval %q: %w", settings.Interval, err)
	}

	effective := effectiveProvider(provider)
	modelName := defaultModel(effective, model)
	if !hasProviderKey(effective) {
		return fmt.Errorf("no API key configured for provider %q. Run 'opsagent config set' first", effective)
	}

	tgToken, tgChat := resolveMonitorTelegram()

	cfg := monitor.Config{
		Interval:       interval,
		CPUThreshold:   settings.CPUThreshold,
		RAMThreshold:   settings.RAMThreshold,
		LogPath:        settings.LogPath,
		TelegramToken:  tgToken,
		TelegramChatID: tgChat,
		Diagnose: func(ctx context.Context, query string) (string, error) {
			return diagnoseQuery(ctx, effective, modelName, query)
		},
	}

	return monitor.Run(context.Background(), cfg)
}

func runMonitorInstall(cmd *cobra.Command, args []string) error {
	settings, err := resolveMonitorSettings(cmd)
	if err != nil {
		return err
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find binary path: %w", err)
	}

	return monitor.Install(monitor.ServiceConfig{
		BinaryPath:   binaryPath,
		Interval:     settings.Interval,
		CPUThreshold: settings.CPUThreshold,
		RAMThreshold: settings.RAMThreshold,
		LogPath:      settings.LogPath,
	})
}

func runMonitorSet(cmd *cobra.Command, args []string) error {
	changed := false

	if cmd.Flags().Changed("interval") {
		if _, err := time.ParseDuration(strings.TrimSpace(monitorSetInterval)); err != nil {
			return fmt.Errorf("invalid --interval %q: %w", monitorSetInterval, err)
		}
		if err := config.Set(monitorIntervalKey, strings.TrimSpace(monitorSetInterval)); err != nil {
			return fmt.Errorf("save %s: %w", monitorIntervalKey, err)
		}
		changed = true
	}
	if cmd.Flags().Changed("cpu-threshold") {
		if monitorSetCPU <= 0 {
			return fmt.Errorf("--cpu-threshold must be > 0")
		}
		if err := config.Set(monitorCPUKey, strconv.Itoa(monitorSetCPU)); err != nil {
			return fmt.Errorf("save %s: %w", monitorCPUKey, err)
		}
		changed = true
	}
	if cmd.Flags().Changed("ram-threshold") {
		if monitorSetRAM <= 0 {
			return fmt.Errorf("--ram-threshold must be > 0")
		}
		if err := config.Set(monitorRAMKey, strconv.Itoa(monitorSetRAM)); err != nil {
			return fmt.Errorf("save %s: %w", monitorRAMKey, err)
		}
		changed = true
	}
	if cmd.Flags().Changed("log") {
		if err := config.Set(monitorLogKey, strings.TrimSpace(monitorSetLog)); err != nil {
			return fmt.Errorf("save %s: %w", monitorLogKey, err)
		}
		changed = true
	}
	if monitorSetClearLog {
		if err := config.Delete(monitorLogKey); err != nil {
			return fmt.Errorf("remove %s: %w", monitorLogKey, err)
		}
		changed = true
	}

	if !changed {
		return fmt.Errorf("no settings provided. Use one or more flags (e.g. --interval 30s --cpu-threshold 85)")
	}

	path, _ := config.FilePath()
	fmt.Printf("Saved monitor defaults in %s\n", path)
	fmt.Println("Use 'opsagent monitor install' to apply defaults to the background service.")
	return nil
}

func runMonitorUninstall(cmd *cobra.Command, args []string) error {
	return monitor.Uninstall()
}

func runMonitorStart(cmd *cobra.Command, args []string) error {
	return monitor.Start()
}

func runMonitorStop(cmd *cobra.Command, args []string) error {
	return monitor.Stop()
}

func runMonitorStatus(cmd *cobra.Command, args []string) error {
	configured, cfgErr := loadMonitorConfigured()

	info, err := monitor.Status()
	if err != nil {
		return err
	}

	fmt.Println("OpsAgent Monitor Status")
	fmt.Println("───────────────────────")
	if cfgErr != nil {
		fmt.Printf("  Config defaults: invalid (%v)\n", cfgErr)
	} else {
		fmt.Printf("  Config defaults:\n")
		fmt.Printf("    Interval:       %s\n", configured.Interval)
		fmt.Printf("    CPU threshold:  %d%%\n", configured.CPUThreshold)
		fmt.Printf("    RAM threshold:  %d%%\n", configured.RAMThreshold)
		if configured.LogPath != "" {
			fmt.Printf("    Log file:       %s\n", configured.LogPath)
		} else {
			fmt.Printf("    Log file:       (none)\n")
		}
	}
	fmt.Println()

	if !info.Installed {
		fmt.Printf("  Installed:  no\n")
		fmt.Printf("  Service:    %s (not found)\n", info.ServiceFile)
		fmt.Println()
		fmt.Println("  Run 'opsagent monitor install' to set up background monitoring from current defaults.")
		return nil
	}

	state := "stopped"
	if info.Running {
		state = "running"
	}
	fmt.Printf("  Installed:      yes\n")
	fmt.Printf("  Status:         %s\n", state)
	fmt.Printf("  Service file:   %s\n", info.ServiceFile)
	if info.BinaryPath != "" {
		fmt.Printf("  Binary:         %s\n", info.BinaryPath)
	}
	if info.Interval != "" {
		fmt.Printf("  Interval:       %s\n", info.Interval)
	}
	if info.CPUThreshold != "" {
		fmt.Printf("  CPU threshold:  %s%%\n", info.CPUThreshold)
	}
	if info.RAMThreshold != "" {
		fmt.Printf("  RAM threshold:  %s%%\n", info.RAMThreshold)
	}
	if info.LogPath != "" {
		fmt.Printf("  Log file:       %s\n", info.LogPath)
	}
	fmt.Println()
	return nil
}

func resolveMonitorSettings(cmd *cobra.Command) (monitorSettings, error) {
	settings := monitorSettings{
		Interval:     "60s",
		CPUThreshold: 90,
		RAMThreshold: 85,
		LogPath:      "",
	}
	configured, err := loadMonitorConfigured()
	if err != nil {
		return settings, err
	}
	if configured.HasInterval {
		settings.Interval = configured.Interval
	}
	if configured.HasCPU {
		settings.CPUThreshold = configured.CPUThreshold
	}
	if configured.HasRAM {
		settings.RAMThreshold = configured.RAMThreshold
	}
	if configured.HasLog {
		settings.LogPath = configured.LogPath
	}

	if cmd.Flags().Changed("interval") {
		settings.Interval = monitorInterval
	}
	if cmd.Flags().Changed("cpu-threshold") {
		settings.CPUThreshold = monitorCPUThreshold
	}
	if cmd.Flags().Changed("ram-threshold") {
		settings.RAMThreshold = monitorRAMThreshold
	}
	if cmd.Flags().Changed("log") {
		settings.LogPath = monitorLogPath
	}
	return settings, nil
}

func loadMonitorConfigured() (monitorConfigured, error) {
	out := monitorConfigured{
		monitorSettings: monitorSettings{
			Interval:     "60s",
			CPUThreshold: 90,
			RAMThreshold: 85,
			LogPath:      "",
		},
	}

	pairs, err := config.Read()
	if err != nil {
		return out, err
	}
	valueFor := func(key string) string {
		val := strings.TrimSpace(pairs[key])
		if val == "" {
			val = strings.TrimSpace(os.Getenv(key))
		}
		return val
	}

	if val := valueFor(monitorIntervalKey); val != "" {
		if _, err := time.ParseDuration(val); err != nil {
			return out, fmt.Errorf("%s: %w", monitorIntervalKey, err)
		}
		out.Interval = val
		out.HasInterval = true
	}
	if val := valueFor(monitorCPUKey); val != "" {
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return out, fmt.Errorf("%s must be a positive integer", monitorCPUKey)
		}
		out.CPUThreshold = n
		out.HasCPU = true
	}
	if val := valueFor(monitorRAMKey); val != "" {
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return out, fmt.Errorf("%s must be a positive integer", monitorRAMKey)
		}
		out.RAMThreshold = n
		out.HasRAM = true
	}
	if val := valueFor(monitorLogKey); val != "" {
		out.LogPath = val
		out.HasLog = true
	}
	return out, nil
}

func resolveMonitorTelegram() (string, int64) {
	token := monitorTGToken
	if token == "" {
		token = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	chatID := monitorTGChatID
	if chatID == 0 {
		if env := os.Getenv("TELEGRAM_CHAT_ID"); env != "" {
			if v, err := strconv.ParseInt(env, 10, 64); err == nil {
				chatID = v
			}
		}
	}
	return token, chatID
}

func diagnoseQuery(ctx context.Context, providerName, modelName, query string) (string, error) {
	a, err := agent.NewAgent(ctx, providerName, modelName)
	if err != nil {
		return "", fmt.Errorf("create agent: %w", err)
	}

	sessionService := session.InMemoryService()
	sess, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "opsagent",
		UserID:  "monitor",
	})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "opsagent",
		Agent:          a,
		SessionService: sessionService,
	})
	if err != nil {
		return "", fmt.Errorf("create runner: %w", err)
	}

	userMsg := &genai.Content{
		Parts: []*genai.Part{genai.NewPartFromText(query)},
		Role:  string(genai.RoleUser),
	}

	var fullText strings.Builder
	for event, err := range r.Run(ctx, "monitor", sess.Session.ID(), userMsg, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeNone,
	}) {
		if err != nil {
			return "", fmt.Errorf("agent run: %w", err)
		}
		if event.Content != nil {
			for _, p := range event.Content.Parts {
				if p.Text != "" {
					fullText.WriteString(p.Text)
				}
			}
		}
	}

	text := strings.TrimSpace(fullText.String())
	if text == "" {
		return "No diagnosis generated.", nil
	}
	return text, nil
}
