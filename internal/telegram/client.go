// Package telegram provides a Telegram Bot API client for OpsAgent.
package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client wraps the Telegram Bot API.
type Client struct {
	Token   string
	BaseURL string
	HTTP    *http.Client
}

// NewClient creates a Telegram API client for the given bot token.
func NewClient(token string) *Client {
	return &Client{
		Token:   token,
		BaseURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Update represents an incoming Telegram update.
type Update struct {
	UpdateID int     `json:"update_id"`
	Message  Message `json:"message"`
}

// Message represents a Telegram message.
type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

// Chat represents a Telegram chat.
type Chat struct {
	ID int64 `json:"id"`
}

type apiResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
	Desc   string          `json:"description"`
}

// GetUpdates fetches new messages using long polling.
func (c *Client) GetUpdates(offset int, timeout int) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", c.BaseURL, offset, timeout)
	resp, err := c.HTTP.Get(url)
	if err != nil {
		return nil, fmt.Errorf("getUpdates: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
		Desc   string   `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode getUpdates: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("getUpdates failed: %s", result.Desc)
	}
	return result.Result, nil
}

// SendMessage sends a text message to a chat.
func (c *Client) SendMessage(chatID int64, text string) error {
	if len(text) > 4096 {
		text = text[:4093] + "..."
	}
	body, _ := json.Marshal(map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})
	resp, err := c.HTTP.Post(c.BaseURL+"/sendMessage", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sendMessage: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Retry without Markdown if parsing failed
		body, _ = json.Marshal(map[string]interface{}{
			"chat_id": chatID,
			"text":    text,
		})
		resp2, err := c.HTTP.Post(c.BaseURL+"/sendMessage", "application/json", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("sendMessage retry: %w", err)
		}
		defer resp2.Body.Close()
		io.Copy(io.Discard, resp2.Body)
	}
	return nil
}

// SetWebhook registers a webhook URL with Telegram.
// If secretToken is non-empty, Telegram will include it in every webhook
// request as the X-Telegram-Bot-Api-Secret-Token header for verification.
func (c *Client) SetWebhook(url, secretToken string) error {
	payload := map[string]string{"url": url}
	if secretToken != "" {
		payload["secret_token"] = secretToken
	}
	body, _ := json.Marshal(payload)
	resp, err := c.HTTP.Post(c.BaseURL+"/setWebhook", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("setWebhook: %w", err)
	}
	defer resp.Body.Close()

	var result apiResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if !result.OK {
		return fmt.Errorf("setWebhook failed: %s", result.Desc)
	}
	return nil
}

// DeleteWebhook removes the webhook so polling works.
func (c *Client) DeleteWebhook() error {
	resp, err := c.HTTP.Post(c.BaseURL+"/deleteWebhook", "application/json", nil)
	if err != nil {
		return fmt.Errorf("deleteWebhook: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// GetWebhookInfo returns the current webhook configuration.
func (c *Client) GetWebhookInfo() (string, error) {
	resp, err := c.HTTP.Get(c.BaseURL + "/getWebhookInfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			URL string `json:"url"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Result.URL, nil
}
