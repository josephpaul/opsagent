package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/josephpaul/opsagent-ai/internal/llmadapter/anthropic"
	"github.com/josephpaul/opsagent-ai/internal/llmadapter/openai"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/genai"
)

const instruction = `You are a DevOps diagnostic assistant. The user will ask about server health, slowness, or resource usage.

Use the available tools to gather data from this machine:
- check_cpu: CPU usage and top process
- check_memory: Memory usage (total, used, free, available)
- check_disk: Disk usage per filesystem
- check_processes: Top processes by CPU

Based on the tool results, provide a clear diagnosis and details. If the user asks 'why is my server slow', call check_cpu, check_memory, and check_disk (and optionally check_processes), then summarize findings (e.g. high CPU, low memory, full disk) and name the main culprits (e.g. top process).

Keep the diagnosis concise and actionable. Format the response as a short summary (diagnosis) followed by details.`

// NewAgent creates an ADK LlmAgent with the given provider and model, and the four diagnostic tools.
// Provider must be "gemini", "openai", or "anthropic". Set the corresponding env: GOOGLE_API_KEY, OPENAI_API_KEY, ANTHROPIC_API_KEY.
func NewAgent(ctx context.Context, provider, modelName string) (agent.Agent, error) {
	tools, err := NewDiagnosticTools()
	if err != nil {
		return nil, fmt.Errorf("diagnostic tools: %w", err)
	}

	llm, err := newLLM(ctx, strings.ToLower(strings.TrimSpace(provider)), modelName)
	if err != nil {
		return nil, err
	}

	return llmagent.New(llmagent.Config{
		Name:        "opsagent_diagnostic_agent",
		Model:       llm,
		Instruction: instruction,
		Description: "Diagnoses this machine by checking CPU, memory, disk, and processes.",
		Tools:       tools,
	})
}

func newLLM(ctx context.Context, provider, modelName string) (model.LLM, error) {
	switch provider {
	case "gemini":
		apiKey := os.Getenv("GOOGLE_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("GOOGLE_API_KEY is not set (required for provider gemini)")
		}
		return gemini.NewModel(ctx, modelName, &genai.ClientConfig{APIKey: apiKey})
	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is not set (required for provider openai)")
		}
		baseURL := os.Getenv("OPENAI_BASE_URL")
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		return openai.NewModel(modelName, &openai.Config{BaseURL: baseURL, APIKey: apiKey})
	case "anthropic":
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set (required for provider anthropic)")
		}
		return anthropic.NewModel(modelName, &anthropic.Config{APIKey: apiKey})
	default:
		return nil, fmt.Errorf("unknown provider %q (use gemini, openai, or anthropic)", provider)
	}
}
