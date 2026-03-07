package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/josephpaul/opsagent-ai/agent"
	"github.com/spf13/cobra"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// effectiveProvider returns the provider whose API key is set. If the requested provider
// has a key, use it; otherwise use the first provider that has a key set (gemini, openai, anthropic).
func effectiveProvider(requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	hasKey := func(p string) bool {
		switch p {
		case "gemini":
			return os.Getenv("GOOGLE_API_KEY") != ""
		case "openai":
			return os.Getenv("OPENAI_API_KEY") != ""
		case "anthropic":
			return os.Getenv("ANTHROPIC_API_KEY") != ""
		}
		return false
	}
	if hasKey(requested) {
		return requested
	}
	for _, p := range []string{"gemini", "openai", "anthropic"} {
		if hasKey(p) {
			return p
		}
	}
	return requested
}

func defaultModel(provider, modelFlag string) string {
	if modelFlag != "" {
		return modelFlag
	}
	switch strings.ToLower(provider) {
	case "openai":
		return "gpt-4o"
	case "anthropic":
		return "claude-sonnet-4-20250514"
	default:
		return "gemini-2.5-flash"
	}
}

func runQuery(cmd *cobra.Command, args []string) error {
	question := args[0]
	ctx := context.Background()
	effective := effectiveProvider(provider)
	modelName := defaultModel(effective, model)

	a, err := agent.NewAgent(ctx, effective, modelName)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	sessionService := session.InMemoryService()
	sess, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "opsagent-ai",
		UserID:  "cli",
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "opsagent-ai",
		Agent:          a,
		SessionService: sessionService,
	})
	if err != nil {
		return fmt.Errorf("create runner: %w", err)
	}

	userMsg := &genai.Content{
		Parts: []*genai.Part{
			genai.NewPartFromText(question),
		},
		Role: string(genai.RoleUser),
	}

	var fullText strings.Builder
	for event, err := range r.Run(ctx, "cli", sess.Session.ID(), userMsg, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeNone,
	}) {
		if err != nil {
			return fmt.Errorf("agent run: %w", err)
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
		text = "No diagnosis generated."
	}
	lines := strings.Split(text, "\n")
	diagnosis := lines[0]
	details := ""
	if len(lines) > 1 {
		details = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}

	fmt.Println("--- Diagnosis ---")
	fmt.Println(diagnosis)
	if details != "" {
		fmt.Println()
		fmt.Println("--- Details ---")
		fmt.Println(details)
	}
	fmt.Fprintln(os.Stdout)
	return nil
}
