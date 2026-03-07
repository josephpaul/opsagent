// Package cmd defines the Cobra commands for the opsagent-ai CLI.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const (
	// DefaultAIAgentURL is the default base URL for the AI agent API.
	DefaultAIAgentURL = "http://localhost:9000"
)

var (
	aiAgentURL string
	provider   string
	model      string
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:   "opsagent-ai [query]",
	Short: "Diagnose Linux servers using natural language",
	Long: `opsagent-ai is a CLI tool that allows you to diagnose Linux servers using natural language.

Example usage:
  opsagent-ai "check cpu"
  opsagent-ai "why is my server slow"
  opsagent-ai "check disk usage"

The CLI sends your query to the AI agent which runs diagnostics via the server agent
and returns a human-readable diagnosis. Use --provider (gemini|anthropic|openai) and
optionally --model to choose which LLM to use.`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&aiAgentURL,
		"agent-url",
		DefaultAIAgentURL,
		"Base URL of the AI agent API (e.g. http://localhost:9000)",
	)
	rootCmd.PersistentFlags().StringVar(
		&provider,
		"provider",
		"",
		"AI provider: gemini | anthropic | openai (default from server env)",
	)
	rootCmd.PersistentFlags().StringVar(
		&model,
		"model",
		"",
		"Model override (e.g. gpt-4o, claude-3-5-sonnet, gemini-2.5-flash)",
	)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
