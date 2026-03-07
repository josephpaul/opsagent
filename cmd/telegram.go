package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/josephpaul/opsagent/internal/telegram"
	"github.com/spf13/cobra"
)

var (
	tgToken         string
	tgChatID        int64
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

Setup:
  1. Create a bot via @BotFather on Telegram to get your bot token
  2. Send a message to your bot, then run:
     opsagent telegram poll --token <TOKEN>
     (it will show your chat ID in the logs)
  3. Optionally restrict to your chat:
     opsagent telegram webhook --token <TOKEN> --chat-id <ID> --webhook-url <URL>

Configure tokens via CLI:
  opsagent config set-key TELEGRAM_BOT_TOKEN <token>
  opsagent config set-key TELEGRAM_CHAT_ID <chat_id>`,
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

func init() {
	telegramCmd.PersistentFlags().StringVar(&tgToken, "token", "", "Telegram bot token (or set TELEGRAM_BOT_TOKEN)")
	telegramCmd.PersistentFlags().Int64Var(&tgChatID, "chat-id", 0, "Restrict to this chat ID (or set TELEGRAM_CHAT_ID)")

	telegramWebhookCmd.Flags().IntVar(&tgPort, "port", 8443, "Port for the webhook HTTP server")
	telegramWebhookCmd.Flags().StringVar(&tgWebhookURL, "webhook-url", "", "Public URL where Telegram sends updates (required)")
	telegramWebhookCmd.Flags().StringVar(&tgWebhookSecret, "webhook-secret", "", "Secret token for webhook verification (or set TELEGRAM_WEBHOOK_SECRET; auto-generated if empty)")

	telegramCmd.AddCommand(telegramWebhookCmd)
	telegramCmd.AddCommand(telegramPollCmd)
	rootCmd.AddCommand(telegramCmd)
}

func resolveTelegramConfig() (string, int64, error) {
	token := tgToken
	if token == "" {
		token = os.Getenv("TELEGRAM_BOT_TOKEN")
	}
	if token == "" {
		return "", 0, fmt.Errorf("bot token required: use --token or 'opsagent config set-key TELEGRAM_BOT_TOKEN <token>'")
	}

	chatID := tgChatID
	if chatID == 0 {
		if env := os.Getenv("TELEGRAM_CHAT_ID"); env != "" {
			parsed, err := strconv.ParseInt(env, 10, 64)
			if err == nil {
				chatID = parsed
			}
		}
	}

	return token, chatID, nil
}

func runTelegramWebhook(cmd *cobra.Command, args []string) error {
	token, chatID, err := resolveTelegramConfig()
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
		AllowedChat:   chatID,
		Diagnose: func(ctx context.Context, query string) (string, error) {
			return diagnoseQuery(ctx, effective, modelName, query)
		},
	})
}

func runTelegramPoll(cmd *cobra.Command, args []string) error {
	token, chatID, err := resolveTelegramConfig()
	if err != nil {
		return err
	}

	effective := effectiveProvider(provider)
	modelName := defaultModel(effective, model)
	if !hasProviderKey(effective) {
		return fmt.Errorf("no API key configured for provider %q. Run 'opsagent config set' first", effective)
	}

	client := telegram.NewClient(token)

	return telegram.RunPoll(context.Background(), telegram.PollConfig{
		Client:      client,
		AllowedChat: chatID,
		Diagnose: func(ctx context.Context, query string) (string, error) {
			return diagnoseQuery(ctx, effective, modelName, query)
		},
	})
}
