package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/josephpaul/opsagent/agent"
	"github.com/josephpaul/opsagent/internal/config"
	"github.com/josephpaul/opsagent/internal/storage"
	"github.com/josephpaul/opsagent/internal/telegram"
	"github.com/spf13/cobra"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/genai"
)

var (
	tgToken         string
	tgUserID        int64
	tgPort          int
	tgWebhookURL    string
	tgWebhookSecret string
)

var telegramCmd = &cobra.Command{
	Use:   "telegram",
	Short: "Run OpsAgent as a Telegram bot",
	Long: `Receive diagnostic queries from Telegram and respond with AI-powered diagnoses.

Webhook mode (recommended for production — Telegram pushes messages to your server):
  opsagent telegram webhook --token <BOT_TOKEN> --webhook-url https://your-server.com

Polling mode (simpler — no public IP needed):
  opsagent telegram poll --token <BOT_TOKEN>

Install as a background service (auto-starts on boot):
  opsagent telegram install --mode webhook --webhook-url https://your-server.com
  opsagent telegram install --mode poll

Manage the service:
  opsagent telegram status / start / stop / uninstall

Setup:
  1. Create a bot via @BotFather on Telegram to get your bot token
  2. Save the token: opsagent config set-key TELEGRAM_BOT_TOKEN <token>
  3. Save your user ID: opsagent config set-key TELEGRAM_USER_ID <your_numeric_id>
  4. Install as a service: opsagent telegram install --mode webhook --webhook-url <URL>

TELEGRAM_USER_ID is required. Without it, no Telegram feature will start.
Only messages from the configured user ID receive responses.`,
}

var telegramWebhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Run in webhook mode (Telegram pushes messages to your server)",
	RunE:  runTelegramWebhook,
}

var telegramPollCmd = &cobra.Command{
	Use:   "poll",
	Short: "Run in polling mode (fetches messages from Telegram)",
	RunE:  runTelegramPoll,
}

var telegramInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the Telegram bot as a background service (systemd/launchd)",
	RunE:  runTelegramInstall,
}

var telegramUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the Telegram bot background service",
	RunE:  runTelegramUninstall,
}

var telegramStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the installed Telegram bot service",
	RunE:  runTelegramStart,
}

var telegramStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Telegram bot service",
	RunE:  runTelegramStop,
}

var telegramStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check if the Telegram bot service is running",
	RunE:  runTelegramStatus,
}

var tgInstallMode string

func init() {
	telegramCmd.PersistentFlags().StringVar(&tgToken, "token", "", "Telegram bot token (or set TELEGRAM_BOT_TOKEN)")
	telegramCmd.PersistentFlags().Int64Var(&tgUserID, "user-id", 0, "Your Telegram user ID (or set TELEGRAM_USER_ID; required)")

	telegramWebhookCmd.Flags().IntVar(&tgPort, "port", 8443, "Port for the webhook HTTP server")
	telegramWebhookCmd.Flags().StringVar(&tgWebhookURL, "webhook-url", "", "Public URL where Telegram sends updates (required)")
	telegramWebhookCmd.Flags().StringVar(&tgWebhookSecret, "webhook-secret", "", "Secret token for webhook verification (or set TELEGRAM_WEBHOOK_SECRET; auto-generated if empty)")

	telegramInstallCmd.Flags().StringVar(&tgInstallMode, "mode", "webhook", "Bot mode: webhook or poll")
	telegramInstallCmd.Flags().StringVar(&tgWebhookURL, "webhook-url", "", "Public URL for webhook mode")
	telegramInstallCmd.Flags().IntVar(&tgPort, "port", 8443, "Port for webhook HTTP server")
	telegramInstallCmd.Flags().StringVar(&tgWebhookSecret, "webhook-secret", "", "Secret token for webhook verification")

	telegramCmd.AddCommand(telegramWebhookCmd)
	telegramCmd.AddCommand(telegramPollCmd)
	telegramCmd.AddCommand(telegramInstallCmd)
	telegramCmd.AddCommand(telegramUninstallCmd)
	telegramCmd.AddCommand(telegramStartCmd)
	telegramCmd.AddCommand(telegramStopCmd)
	telegramCmd.AddCommand(telegramStatusCmd)
	rootCmd.AddCommand(telegramCmd)
}

func resolveTelegramConfig() (token string, userID int64, err error) {
	token = tgToken
	if token == "" {
		token = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	if token == "" {
		return "", 0, fmt.Errorf("bot token required: use --token or 'opsagent config set-key TELEGRAM_BOT_TOKEN <token>'")
	}

	userID = tgUserID
	if userID == 0 {
		if env := os.Getenv("TELEGRAM_USER_ID"); env != "" {
			parsed, err := strconv.ParseInt(env, 10, 64)
			if err == nil {
				userID = parsed
			}
		}
	}
	if userID == 0 {
		return "", 0, fmt.Errorf("TELEGRAM_USER_ID is required: use --user-id or 'opsagent config set-key TELEGRAM_USER_ID <your_id>'")
	}

	return token, userID, nil
}

func runTelegramWebhook(cmd *cobra.Command, args []string) error {
	token, userID, err := resolveTelegramConfig()
	if err != nil {
		return err
	}
	if tgWebhookURL == "" {
		return fmt.Errorf("--webhook-url is required (e.g. https://your-server.com)")
	}

	effective := effectiveProvider(provider)
	modelName := defaultModel(effective, model)
	if !hasProviderKey(effective) {
		return fmt.Errorf("no API key configured for provider %q. Run 'opsagent config set' first", effective)
	}

	store, a, r, err := setupSessionAgent(effective, modelName)
	if err != nil {
		return err
	}
	defer store.Close()

	webhookSecret := tgWebhookSecret
	if webhookSecret == "" {
		webhookSecret = os.Getenv("TELEGRAM_WEBHOOK_SECRET")
	}

	client := telegram.NewClient(token)

	return telegram.RunWebhook(context.Background(), telegram.WebhookConfig{
		Client:        client,
		Port:          tgPort,
		WebhookURL:    tgWebhookURL,
		WebhookSecret: webhookSecret,
		AllowedUserID: userID,
		Diagnose:      sessionDiagnoseFunc(store, a, r),
		Clear:         sessionClearFunc(store),
	})
}

func runTelegramPoll(cmd *cobra.Command, args []string) error {
	token, userID, err := resolveTelegramConfig()
	if err != nil {
		return err
	}

	effective := effectiveProvider(provider)
	modelName := defaultModel(effective, model)
	if !hasProviderKey(effective) {
		return fmt.Errorf("no API key configured for provider %q. Run 'opsagent config set' first", effective)
	}

	store, a, r, err := setupSessionAgent(effective, modelName)
	if err != nil {
		return err
	}
	defer store.Close()

	client := telegram.NewClient(token)

	return telegram.RunPoll(context.Background(), telegram.PollConfig{
		Client:        client,
		AllowedUserID: userID,
		Diagnose:      sessionDiagnoseFunc(store, a, r),
		Clear:         sessionClearFunc(store),
	})
}

func setupSessionAgent(providerName, modelName string) (*storage.SQLiteService, adkagent.Agent, *runner.Runner, error) {
	ctx := context.Background()

	dir, err := config.Dir()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("config dir: %w", err)
	}
	dbPath := filepath.Join(dir, "sessions.db")

	store, err := storage.NewSQLiteService(dbPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open session store: %w", err)
	}

	a, err := agent.NewAgent(ctx, providerName, modelName)
	if err != nil {
		store.Close()
		return nil, nil, nil, fmt.Errorf("create agent: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "opsagent",
		Agent:          a,
		SessionService: store.InnerService(),
	})
	if err != nil {
		store.Close()
		return nil, nil, nil, fmt.Errorf("create runner: %w", err)
	}

	return store, a, r, nil
}

func sessionDiagnoseFunc(store *storage.SQLiteService, a adkagent.Agent, r *runner.Runner) telegram.DiagnoseFunc {
	return func(ctx context.Context, userID, query string) (string, error) {
		sess, err := store.GetOrCreateSession(ctx, "opsagent", userID)
		if err != nil {
			return "", fmt.Errorf("get session: %w", err)
		}

		userMsg := &genai.Content{
			Parts: []*genai.Part{genai.NewPartFromText(query)},
			Role:  string(genai.RoleUser),
		}

		var fullText strings.Builder
		for event, err := range r.Run(ctx, userID, sess.ID(), userMsg, adkagent.RunConfig{
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

		if err := store.PersistNewEvents(ctx, sess); err != nil {
			return "", fmt.Errorf("persist session: %w", err)
		}

		text := strings.TrimSpace(fullText.String())
		if text == "" {
			return "No diagnosis generated.", nil
		}
		return text, nil
	}
}

func sessionClearFunc(store *storage.SQLiteService) telegram.ClearFunc {
	return func(ctx context.Context, userID string) error {
		return store.DeleteUserSessions(ctx, "opsagent", userID)
	}
}

func runTelegramInstall(cmd *cobra.Command, args []string) error {
	mode := tgInstallMode
	if mode != "webhook" && mode != "poll" {
		return fmt.Errorf("--mode must be 'webhook' or 'poll'")
	}
	if mode == "webhook" && tgWebhookURL == "" {
		return fmt.Errorf("--webhook-url is required for webhook mode")
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find binary path: %w", err)
	}

	webhookSecret := tgWebhookSecret
	if webhookSecret == "" {
		webhookSecret = os.Getenv("TELEGRAM_WEBHOOK_SECRET")
	}

	return telegram.InstallService(telegram.ServiceConfig{
		BinaryPath:    binaryPath,
		Mode:          mode,
		WebhookURL:    tgWebhookURL,
		Port:          tgPort,
		WebhookSecret: webhookSecret,
	})
}

func runTelegramUninstall(cmd *cobra.Command, args []string) error {
	return telegram.UninstallService()
}

func runTelegramStart(cmd *cobra.Command, args []string) error {
	return telegram.StartService()
}

func runTelegramStop(cmd *cobra.Command, args []string) error {
	return telegram.StopService()
}

func runTelegramStatus(cmd *cobra.Command, args []string) error {
	info, err := telegram.ServiceStatus()
	if err != nil {
		return err
	}

	fmt.Println("OpsAgent Telegram Bot Status")
	fmt.Println("────────────────────────────")

	if !info.Installed {
		fmt.Printf("  Installed:    no\n")
		fmt.Printf("  Service:      %s (not found)\n", info.ServiceFile)
		fmt.Println()
		fmt.Println("  Run 'opsagent telegram install --mode webhook --webhook-url <URL>' to set up.")
		return nil
	}

	state := "stopped"
	if info.Running {
		state = "running"
	}
	fmt.Printf("  Installed:    yes\n")
	fmt.Printf("  Status:       %s\n", state)
	fmt.Printf("  Service file: %s\n", info.ServiceFile)
	if info.BinaryPath != "" {
		fmt.Printf("  Binary:       %s\n", info.BinaryPath)
	}
	if info.Mode != "" {
		fmt.Printf("  Mode:         %s\n", info.Mode)
	}
	if info.WebhookURL != "" {
		fmt.Printf("  Webhook URL:  %s\n", info.WebhookURL)
	}
	if info.Port != "" {
		fmt.Printf("  Port:         %s\n", info.Port)
	}
	fmt.Println()
	return nil
}
