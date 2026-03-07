// Package openai implements the ADK model.LLM interface for OpenAI-compatible APIs
// (OpenAI API and any endpoint that accepts OpenAI chat completion format).
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// Config configures the OpenAI-compatible adapter.
type Config struct {
	BaseURL string // e.g. "https://api.openai.com/v1"
	APIKey  string
	Client  *http.Client
}

// Model implements model.LLM by calling an OpenAI-compatible chat completions API.
type Model struct {
	name   string
	base   string
	apiKey string
	client *http.Client
}

// NewModel returns an LLM that uses the OpenAI-compatible API at baseURL.
func NewModel(modelName string, cfg *Config) (model.LLM, error) {
	if cfg == nil || cfg.BaseURL == "" {
		return nil, fmt.Errorf("openai: config with BaseURL is required")
	}
	base := strings.TrimSuffix(cfg.BaseURL, "/")
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &Model{
		name:   modelName,
		base:   base,
		apiKey: cfg.APIKey,
		client: client,
	}, nil
}

// Name implements model.LLM.
func (m *Model) Name() string {
	return m.name
}

// GenerateContent implements model.LLM. It converts genai request to OpenAI format,
// calls the API, and converts the response back.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if stream {
		return m.generateStream(ctx, req)
	}
	return m.generateNonStream(ctx, req)
}

func (m *Model) generateNonStream(ctx context.Context, req *model.LLMRequest) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		out, err := m.callAPI(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(out, nil)
	}
}

func (m *Model) generateStream(ctx context.Context, req *model.LLMRequest) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		out, err := m.callAPIStream(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		for _, r := range out {
			if !yield(r, nil) {
				return
			}
		}
	}
}

// openaiReq is the request body for POST /chat/completions.
type openaiReq struct {
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
	Tools    []openaiTool    `json:"tools,omitempty"`
	Stream   bool            `json:"stream,omitempty"`
	MaxTokens *int           `json:"max_tokens,omitempty"`
}

type openaiMessage struct {
	Role       string          `json:"role"`
	Content    interface{}     `json:"content,omitempty"` // string or []contentPart
	ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

type openaiContentPart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ToolCall *struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	} `json:"tool_call,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string      `json:"name"`
		Description string      `json:"description,omitempty"`
		Parameters  interface{} `json:"parameters,omitempty"`
	} `json:"function"`
}

type openaiResp struct {
	Choices []struct {
		Message struct {
			Role       string          `json:"role"`
			Content    string          `json:"content"`
			ToolCalls  []openaiToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

const maxRetries = 3

func (m *Model) callAPI(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	messages, err := contentsToOpenAIMessages(req.Contents)
	if err != nil {
		return nil, err
	}
	tools := configToolsToOpenAI(req.Config)
	modelName := req.Model
	if modelName == "" {
		modelName = m.name
	}
	maxTok := 4096
	if req.Config != nil && req.Config.MaxOutputTokens > 0 {
		maxTok = int(req.Config.MaxOutputTokens)
	}
	body := openaiReq{
		Model:     modelName,
		Messages:  messages,
		Tools:     tools,
		MaxTokens: &maxTok,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Brief pause before retry; increases with each attempt.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
			}
		}
		httpReq, err := http.NewRequestWithContext(ctx, "POST", m.base+"/chat/completions", bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("openai: new request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if m.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+m.apiKey)
		}
		resp, err := m.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("openai: do request: %w", err)
			continue // retry on connection errors (GOAWAY, EOF, etc.)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			bs, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("openai: %s: %s", resp.Status, string(bs))
			continue // retry on rate-limit or server errors
		}
		if resp.StatusCode != http.StatusOK {
			bs, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("openai: %s: %s", resp.Status, string(bs))
		}
		var oai openaiResp
		if err := json.NewDecoder(resp.Body).Decode(&oai); err != nil {
			return nil, fmt.Errorf("openai: decode response: %w", err)
		}
		if oai.Error != nil {
			return &model.LLMResponse{ErrorMessage: oai.Error.Message, ErrorCode: oai.Error.Type}, nil
		}
		if len(oai.Choices) == 0 {
			return nil, fmt.Errorf("openai: empty choices")
		}
		choice := &oai.Choices[0]
		content := openAIMessageToGenaiContent(choice.Message.Role, choice.Message.Content, choice.Message.ToolCalls)
		return &model.LLMResponse{
			Content:      content,
			FinishReason: genai.FinishReason(choice.FinishReason),
		}, nil
	}
	return nil, lastErr
}

func (m *Model) callAPIStream(ctx context.Context, req *model.LLMRequest) ([]*model.LLMResponse, error) {
	// Non-streaming only for simplicity; streaming would require SSE parsing.
	return nil, fmt.Errorf("openai: streaming not implemented")
}

func contentsToOpenAIMessages(contents []*genai.Content) ([]openaiMessage, error) {
	var out []openaiMessage
	for _, c := range contents {
		if c == nil {
			continue
		}
		role := c.Role
		if role == "" {
			role = "user"
		}
		var textParts []string
		var toolCalls []openaiToolCall
		var toolResponses []openaiMessage
		for _, p := range c.Parts {
			if p == nil {
				continue
			}
			if p.Text != "" {
				textParts = append(textParts, p.Text)
				continue
			}
			if p.FunctionCall != nil {
				args := ""
				if p.FunctionCall.Args != nil {
					b, _ := json.Marshal(p.FunctionCall.Args)
					args = string(b)
				}
				toolCalls = append(toolCalls, openaiToolCall{
					ID:   p.FunctionCall.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: p.FunctionCall.Name, Arguments: args},
				})
				continue
			}
			if p.FunctionResponse != nil {
				respJSON, _ := json.Marshal(p.FunctionResponse.Response)
				toolCallID := p.FunctionResponse.ID
				if toolCallID == "" {
					toolCallID = p.FunctionResponse.Name
				}
				toolResponses = append(toolResponses, openaiMessage{
					Role:       "tool",
					ToolCallID: toolCallID,
					Content:    string(respJSON),
				})
				continue
			}
		}
		for _, tr := range toolResponses {
			out = append(out, tr)
		}
		var content interface{}
		if len(textParts) > 0 {
			content = strings.Join(textParts, "\n")
		}
		if content != nil || len(toolCalls) > 0 {
			out = append(out, openaiMessage{Role: role, Content: content, ToolCalls: toolCalls})
		}
	}
	return out, nil
}

func openAIMessageToGenaiContent(role, text string, toolCalls []openaiToolCall) *genai.Content {
	var parts []*genai.Part
	if text != "" {
		parts = append(parts, genai.NewPartFromText(text))
	}
	for _, tc := range toolCalls {
		var args map[string]any
		if tc.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
		}
		part := genai.NewPartFromFunctionCall(tc.Function.Name, args)
		// Preserve the tool call ID so the ADK can use it in tool results; OpenAI requires tool_call_id to match.
		if part.FunctionCall != nil && tc.ID != "" {
			part.FunctionCall.ID = tc.ID
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		return &genai.Content{Role: role}
	}
	return &genai.Content{Parts: parts, Role: role}
}

func configToolsToOpenAI(cfg *genai.GenerateContentConfig) []openaiTool {
	if cfg == nil || len(cfg.Tools) == 0 {
		return nil
	}
	var list []openaiTool
	for _, t := range cfg.Tools {
		if t == nil {
			continue
		}
		for _, fd := range t.FunctionDeclarations {
			if fd == nil {
				continue
			}
			var params interface{} = emptyObject
			if fd.Parameters != nil {
				params = fd.Parameters
			}
			list = append(list, openaiTool{
				Type: "function",
				Function: struct {
					Name        string      `json:"name"`
					Description string      `json:"description,omitempty"`
					Parameters  interface{} `json:"parameters,omitempty"`
				}{
					Name:        fd.Name,
					Description: fd.Description,
					Parameters:  params,
				},
			})
		}
	}
	return list
}

var emptyObject = map[string]any{}
