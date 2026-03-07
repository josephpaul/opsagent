package telegram

import (
	"context"
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

// DiagnoseFunc is the function signature for running an AI diagnosis.
type DiagnoseFunc func(ctx context.Context, query string) (string, error)

// WebhookConfig holds the settings for the webhook server.
type WebhookConfig struct {
	Client      *Client
	Port        int
	WebhookURL  string
	AllowedChat int64 // 0 = allow all chats
	Diagnose    DiagnoseFunc
}

// RunWebhook starts an HTTP server that receives Telegram webhook pushes.
// It registers the webhook with Telegram, handles incoming messages, and
// shuts down gracefully on SIGINT/SIGTERM.
func RunWebhook(ctx context.Context, cfg WebhookConfig) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	path := "/webhook/telegram"
	fullURL := strings.TrimRight(cfg.WebhookURL, "/") + path

	if err := cfg.Client.SetWebhook(fullURL); err != nil {
		return fmt.Errorf("register webhook: %w", err)
	}
	log.Printf("[telegram] webhook registered: %s", fullURL)

	handler := &messageHandler{
		client:      cfg.Client,
		allowedChat: cfg.AllowedChat,
		diagnose:    cfg.Diagnose,
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	Client      *Client
	AllowedChat int64
	Diagnose    DiagnoseFunc
}

// RunPoll starts a long-polling loop that fetches messages from Telegram.
func RunPoll(ctx context.Context, cfg PollConfig) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg.Client.DeleteWebhook()

	handler := &messageHandler{
		client:      cfg.Client,
		allowedChat: cfg.AllowedChat,
		diagnose:    cfg.Diagnose,
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
	client      *Client
	allowedChat int64
	diagnose    DiagnoseFunc
}

func (h *messageHandler) handle(ctx context.Context, update Update) {
	chatID := update.Message.Chat.ID
	text := strings.TrimSpace(update.Message.Text)

	if text == "" {
		return
	}

	if h.allowedChat != 0 && chatID != h.allowedChat {
		log.Printf("[telegram] ignored message from unauthorized chat %d", chatID)
		return
	}

	if text == "/start" {
		h.client.SendMessage(chatID, "Welcome to OpsAgent! Send me a diagnostic query like:\n\n"+
			"• check cpu\n• why is my server slow\n• check disk usage")
		return
	}

	if text == "/status" {
		h.client.SendMessage(chatID, "OpsAgent is running and ready for queries.")
		return
	}

	log.Printf("[telegram] query from chat %d: %s", chatID, text)
	h.client.SendMessage(chatID, "Diagnosing...")

	diagnosis, err := h.diagnose(ctx, text)
	if err != nil {
		log.Printf("[telegram] diagnosis error: %v", err)
		h.client.SendMessage(chatID, fmt.Sprintf("Error: %v", err))
		return
	}

	log.Printf("[telegram] diagnosis sent to chat %d", chatID)
	h.client.SendMessage(chatID, diagnosis)
}

// Notify sends a one-off message to a chat (used by the monitor for alerts).
func Notify(token string, chatID int64, message string) error {
	client := NewClient(token)
	return client.SendMessage(chatID, message)
}
