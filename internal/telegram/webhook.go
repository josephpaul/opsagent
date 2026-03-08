package telegram

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// DiagnoseFunc runs an AI diagnosis. The userID identifies the conversation
// (e.g. "tg-123456" for Telegram chats) so the session store can maintain
// per-user memory.
type DiagnoseFunc func(ctx context.Context, userID, query string) (string, error)

// ClearFunc clears the conversation history for a user.
type ClearFunc func(ctx context.Context, userID string) error

// WebhookConfig holds the settings for the webhook server.
type WebhookConfig struct {
	Client        *Client
	Port          int
	WebhookURL    string
	WebhookSecret string // if empty, a random token is generated per startup
	AllowedUserID int64  // only respond to messages from this Telegram user ID
	Diagnose      DiagnoseFunc
	Clear         ClearFunc
}

// RunWebhook starts an HTTP server that receives Telegram webhook pushes.
// It registers the webhook with Telegram, handles incoming messages, and
// shuts down gracefully on SIGINT/SIGTERM.
func RunWebhook(ctx context.Context, cfg WebhookConfig) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	secretToken := cfg.WebhookSecret
	if secretToken == "" {
		var err error
		secretToken, err = generateSecretToken()
		if err != nil {
			return fmt.Errorf("generate secret token: %w", err)
		}
		log.Println("[telegram] using auto-generated webhook secret (set TELEGRAM_WEBHOOK_SECRET for a persistent one)")
	}

	path := "/webhook/telegram"
	fullURL := strings.TrimRight(cfg.WebhookURL, "/") + path

	if err := cfg.Client.SetWebhook(fullURL, secretToken); err != nil {
		return fmt.Errorf("register webhook: %w", err)
	}
	log.Printf("[telegram] webhook registered: %s (with secret token verification)", fullURL)

	handler := &messageHandler{
		client:        cfg.Client,
		allowedUserID: cfg.AllowedUserID,
		diagnose:      cfg.Diagnose,
		clear:         cfg.Clear,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != secretToken {
			log.Printf("[telegram] rejected request: invalid secret token from %s", r.RemoteAddr)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var update Update
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)

		go handler.handle(ctx, update)
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		log.Println("[telegram] shutting down webhook server...")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		server.Shutdown(shutCtx)
	}()

	log.Printf("[telegram] webhook server listening on :%d%s", cfg.Port, path)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("webhook server: %w", err)
	}
	return nil
}

// PollConfig holds the settings for the polling loop.
type PollConfig struct {
	Client        *Client
	AllowedUserID int64
	Diagnose      DiagnoseFunc
	Clear         ClearFunc
}

// RunPoll starts a long-polling loop that fetches messages from Telegram.
func RunPoll(ctx context.Context, cfg PollConfig) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg.Client.DeleteWebhook()

	handler := &messageHandler{
		client:        cfg.Client,
		allowedUserID: cfg.AllowedUserID,
		diagnose:      cfg.Diagnose,
		clear:         cfg.Clear,
	}

	log.Println("[telegram] polling for messages (Ctrl+C to stop)...")
	offset := 0

	for {
		select {
		case <-ctx.Done():
			log.Println("[telegram] polling stopped")
			return nil
		default:
		}

		updates, err := cfg.Client.GetUpdates(offset, 30)
		if err != nil {
			log.Printf("[telegram] poll error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			offset = update.UpdateID + 1
			go handler.handle(ctx, update)
		}
	}
}

type messageHandler struct {
	client        *Client
	allowedUserID int64
	diagnose      DiagnoseFunc
	clear         ClearFunc
}

func (h *messageHandler) handle(ctx context.Context, update Update) {
	chatID := update.Message.Chat.ID
	text := strings.TrimSpace(update.Message.Text)

	if text == "" {
		return
	}

	var senderID int64
	if update.Message.From != nil {
		senderID = update.Message.From.ID
	}

	if h.allowedUserID != 0 && senderID != h.allowedUserID {
		return
	}

	userID := fmt.Sprintf("tg-%d", senderID)

	if text == "/start" {
		h.client.SendMessage(chatID, "Welcome to OpsAgent! Send me a diagnostic query like:\n\n"+
			"• check cpu\n• why is my server slow\n• check disk usage\n\n"+
			"Use /clear to reset conversation history.")
		return
	}

	if text == "/status" {
		h.client.SendMessage(chatID, "OpsAgent is running and ready for queries.")
		return
	}

	if text == "/clear" {
		if h.clear != nil {
			if err := h.clear(ctx, userID); err != nil {
				log.Printf("[telegram] clear error for %s: %v", userID, err)
				h.client.SendMessage(chatID, fmt.Sprintf("Error clearing history: %v", err))
				return
			}
		}
		h.client.SendMessage(chatID, "Conversation history cleared.")
		return
	}

	log.Printf("[telegram] query from chat %d: %s", chatID, text)
	h.client.SendMessage(chatID, "Diagnosing...")

	diagnosis, err := h.diagnose(ctx, userID, text)
	if err != nil {
		log.Printf("[telegram] diagnosis error: %v", err)
		h.client.SendMessage(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	log.Printf("[telegram] diagnosis sent to chat %d", chatID)
	h.client.SendMessage(chatID, diagnosis)
}

// generateSecretToken creates a cryptographically random 32-byte hex string.
// Telegram accepts 1-256 character tokens containing A-Za-z0-9_-.
func generateSecretToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Notify sends a one-off message to a chat (used by the monitor for alerts).
func Notify(token string, chatID int64, message string) error {
	client := NewClient(token)
	return client.SendMessage(chatID, message)
}
