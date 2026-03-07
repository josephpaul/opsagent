package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/josephpaul/opsagent/agent"
	"github.com/spf13/cobra"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func hasProviderKey(p string) bool {
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

// effectiveProvider returns the best provider choice based on the explicit flag,
// configured default provider, and whichever API keys are available.
func effectiveProvider(requested string) string {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if hasProviderKey(requested) {
		return requested
	}

	preferred := strings.ToLower(strings.TrimSpace(os.Getenv("OPSAGENT_PROVIDER")))
	if requested == "" && hasProviderKey(preferred) {
		return preferred
	}

	for _, p := range []string{"gemini", "openai", "anthropic"} {
		if hasProviderKey(p) {
			return p
		}
	}

	if requested != "" {
		return requested
	}
	if preferred != "" {
		return preferred
	}
	return "gemini"
}

func defaultModel(provider, modelFlag string) string {
	if modelFlag != "" {
		return modelFlag
	}
	if configured := strings.TrimSpace(os.Getenv("OPSAGENT_MODEL")); configured != "" {
		return configured
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

	if !hasProviderKey(effective) {
		return fmt.Errorf("no API key configured for provider %q. Run 'opsagent config set' or 'opsagent config set-key' first", effective)
	}

	a, err := agent.NewAgent(ctx, effective, modelName)
	if err != nil {
		return fmt.Errorf("create agent: %w", err)
	}

	sessionService := session.InMemoryService()
	sess, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: "opsagent",
		UserID:  "cli",
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	r, err := runner.New(runner.Config{
		AppName:        "opsagent",
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
