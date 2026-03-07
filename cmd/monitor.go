package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/josephpaul/opsagent/agent"
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
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Background monitoring with automatic AI diagnosis on threshold breaches",
	Long: `Run a background monitor that polls CPU and RAM usage at a regular interval.
When usage exceeds the configured thresholds, the AI agent is automatically
invoked to diagnose the cause.

  opsagent monitor run                  Run in foreground (Ctrl+C to stop)
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

	monitorCmd.AddCommand(monitorRunCmd)
	monitorCmd.AddCommand(monitorInstallCmd)
	monitorCmd.AddCommand(monitorUninstallCmd)
	monitorCmd.AddCommand(monitorStartCmd)
	monitorCmd.AddCommand(monitorStopCmd)
	monitorCmd.AddCommand(monitorStatusCmd)
	rootCmd.AddCommand(monitorCmd)
}

func runMonitor(cmd *cobra.Command, args []string) error {
	interval, err := time.ParseDuration(monitorInterval)
	if err != nil {
		return fmt.Errorf("invalid interval %q: %w", monitorInterval, err)
	}

	effective := effectiveProvider(provider)
	modelName := defaultModel(effective, model)
	if !hasProviderKey(effective) {
		return fmt.Errorf("no API key configured for provider %q. Run 'opsagent config set' first", effective)
	}

	tgToken, tgChat := resolveMonitorTelegram()

	cfg := monitor.Config{
		Interval:       interval,
		CPUThreshold:   monitorCPUThreshold,
		RAMThreshold:   monitorRAMThreshold,
		LogPath:        monitorLogPath,
		TelegramToken:  tgToken,
		TelegramChatID: tgChat,
		Diagnose: func(ctx context.Context, query string) (string, error) {
			return diagnoseQuery(ctx, effective, modelName, query)
		},
	}

	return monitor.Run(context.Background(), cfg)
}

func runMonitorInstall(cmd *cobra.Command, args []string) error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find binary path: %w", err)
	}

	return monitor.Install(monitor.ServiceConfig{
		BinaryPath:   binaryPath,
		Interval:     monitorInterval,
		CPUThreshold: monitorCPUThreshold,
		RAMThreshold: monitorRAMThreshold,
		LogPath:      monitorLogPath,
	})
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
	info, err := monitor.Status()
	if err != nil {
		return err
	}

	fmt.Println("OpsAgent Monitor Status")
	fmt.Println("───────────────────────")

	if !info.Installed {
		fmt.Printf("  Installed:  no\n")
		fmt.Printf("  Service:    %s (not found)\n", info.ServiceFile)
		fmt.Println()
		fmt.Println("  Run 'opsagent monitor install' to set up background monitoring.")
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
