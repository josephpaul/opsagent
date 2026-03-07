// Package anthropic implements the ADK model.LLM interface for the Anthropic Messages API.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

const defaultAnthropicVersion = "2023-06-01"

// Config configures the Anthropic adapter.
type Config struct {
	APIKey string
	Client *http.Client
}

// Model implements model.LLM by calling the Anthropic Messages API.
type Model struct {
	name   string
	apiKey string
	client *http.Client
}

// NewModel returns an LLM that uses the Anthropic API.
func NewModel(modelName string, cfg *Config) (model.LLM, error) {
	if cfg == nil || cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: config with APIKey is required")
	}
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	return &Model{
		name:   modelName,
		apiKey: cfg.APIKey,
		client: client,
	}, nil
}

// Name implements model.LLM.
func (m *Model) Name() string {
	return m.name
}

// GenerateContent implements model.LLM.
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
		out, err := m.callAPI(ctx, req)
		if err != nil {
			yield(nil, err)
			return
		}
		yield(out, nil)
	}
}

type anthropicReq struct {
	Model      string            `json:"model"`
	MaxTokens  int               `json:"max_tokens"`
	Messages   []anthropicMsg    `json:"messages"`
	Tools      []anthropicTool   `json:"tools,omitempty"`
	System     string            `json:"system,omitempty"`
}

type anthropicMsg struct {
	Role    string        `json:"role"`
	Content anthropicContent `json:"content"`
}

// content can be string or array of blocks
type anthropicContent []anthropicBlock

func (c anthropicContent) MarshalJSON() ([]byte, error) {
	if len(c) == 1 && c[0].Type == "text" && c[0].Text != "" {
		return json.Marshal(c[0].Text)
	}
	return json.Marshal([]anthropicBlock(c))
}

type anthropicBlock struct {
	Type       string      `json:"type,omitempty"`
	Text       string      `json:"text,omitempty"`
	ID         string      `json:"id,omitempty"`
	Name       string      `json:"name,omitempty"`
	Input      interface{} `json:"input,omitempty"`
	ToolUseID  string      `json:"tool_use_id,omitempty"`
	Content    string      `json:"content,omitempty"`
}

type anthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicResp struct {
	Content []struct {
		Type  string      `json:"type"`
		Text  string      `json:"text,omitempty"`
		ID    string      `json:"id,omitempty"`
		Name  string      `json:"name,omitempty"`
		Input interface{} `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (m *Model) callAPI(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	messages, system, err := contentsToAnthropicMessages(req.Contents)
	if err != nil {
		return nil, err
	}
	tools := configToolsToAnthropic(req.Config)
	modelName := req.Model
	if modelName == "" {
		modelName = m.name
	}
	maxTok := 4096
	if req.Config != nil && req.Config.MaxOutputTokens > 0 {
		maxTok = int(req.Config.MaxOutputTokens)
	}
	body := anthropicReq{
		Model:     modelName,
		MaxTokens: maxTok,
		Messages:  messages,
		Tools:     tools,
		System:    system,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("anthropic: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", defaultAnthropicVersion)
	httpReq.Header.Set("x-api-key", m.apiKey)
	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bs, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic: %s: %s", resp.Status, string(bs))
	}
	var out anthropicResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w", err)
	}
	if out.Error != nil {
		return &model.LLMResponse{ErrorMessage: out.Error.Message, ErrorCode: out.Error.Type}, nil
	}
	content := anthropicContentToGenai(out.Content, out.StopReason)
	return &model.LLMResponse{
		Content:      content,
		FinishReason: anthropicStopReasonToGenai(out.StopReason),
	}, nil
}

func contentsToAnthropicMessages(contents []*genai.Content) ([]anthropicMsg, string, error) {
	var messages []anthropicMsg
	var system string
	for _, c := range contents {
		if c == nil {
			continue
		}
		role := c.Role
		if role == "" {
			role = "user"
		}
		if strings.ToLower(role) == "model" {
			role = "assistant"
		}
		if strings.ToLower(role) == "system" {
			for _, p := range c.Parts {
				if p != nil && p.Text != "" {
					system = p.Text
					break
				}
			}
			continue
		}
		var blocks []anthropicBlock
		for _, p := range c.Parts {
			if p == nil {
				continue
			}
			if p.Text != "" {
				blocks = append(blocks, anthropicBlock{Type: "text", Text: p.Text})
				continue
			}
			if p.FunctionCall != nil {
				blocks = append(blocks, anthropicBlock{
					Type:  "tool_use",
					ID:    p.FunctionCall.ID,
					Name:  p.FunctionCall.Name,
					Input: p.FunctionCall.Args,
				})
				continue
			}
			if p.FunctionResponse != nil {
				contentStr := ""
				if p.FunctionResponse.Response != nil {
					b, _ := json.Marshal(p.FunctionResponse.Response)
					contentStr = string(b)
				}
				toolUseID := p.FunctionResponse.ID
				if toolUseID == "" {
					toolUseID = p.FunctionResponse.Name
				}
				blocks = append(blocks, anthropicBlock{
					Type:      "tool_result",
					ToolUseID: toolUseID,
					Content:   contentStr,
				})
				continue
			}
		}
		if len(blocks) > 0 {
			messages = append(messages, anthropicMsg{Role: role, Content: blocks})
		}
	}
	return messages, system, nil
}

func anthropicContentToGenai(blocks []struct {
	Type  string      `json:"type"`
	Text  string      `json:"text,omitempty"`
	ID    string      `json:"id,omitempty"`
	Name  string      `json:"name,omitempty"`
	Input interface{} `json:"input,omitempty"`
}, stopReason string) *genai.Content {
	var parts []*genai.Part
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, genai.NewPartFromText(b.Text))
			continue
		}
		if b.Type == "tool_use" {
			args, _ := b.Input.(map[string]any)
			if args == nil {
				args = make(map[string]any)
			}
			parts = append(parts, genai.NewPartFromFunctionCall(b.Name, args))
		}
	}
	if len(parts) == 0 {
		return &genai.Content{Role: "model"}
	}
	return &genai.Content{Parts: parts, Role: "model"}
}

func anthropicStopReasonToGenai(s string) genai.FinishReason {
	switch s {
	case "end_turn":
		return genai.FinishReasonStop
	case "tool_use":
		return genai.FinishReason("TOOL_USE")
	case "max_tokens":
		return genai.FinishReasonMaxTokens
	default:
		return genai.FinishReason(s)
	}
}

func configToolsToAnthropic(cfg *genai.GenerateContentConfig) []anthropicTool {
	if cfg == nil || len(cfg.Tools) == 0 {
		return nil
	}
	var list []anthropicTool
	for _, t := range cfg.Tools {
		if t == nil {
			continue
		}
		for _, fd := range t.FunctionDeclarations {
			if fd == nil {
				continue
			}
			schema := schemaFromGenai(fd.Parameters)
			list = append(list, anthropicTool{
				Name:        fd.Name,
				Description: fd.Description,
				InputSchema: schema,
			})
		}
	}
	return list
}

func schemaFromGenai(s *genai.Schema) map[string]interface{} {
	if s == nil {
		return map[string]interface{}{"type": "object"}
	}
	// Marshal genai.Schema to JSON and unmarshal to map for Anthropic's input_schema.
	b, err := json.Marshal(s)
	if err != nil {
		return map[string]interface{}{"type": "object"}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]interface{}{"type": "object"}
	}
	if out["type"] == nil || out["type"] == "" {
		out["type"] = "object"
	}
	return out
}
