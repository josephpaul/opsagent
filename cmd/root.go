package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var provider string
var model string

var rootCmd = &cobra.Command{
	Use:   "opsagent-ai [query]",
	Short: "Diagnose this machine using natural language",
	Long: `opsagent-ai is a CLI that diagnoses the current machine using an AI agent and local diagnostics.

Example usage:
  opsagent-ai "check cpu"
  opsagent-ai "why is my server slow"
  opsagent-ai --provider openai --model gpt-4o "check disk usage"
  opsagent-ai --provider anthropic --model claude-sonnet-4-20250514 "check memory"

Set the API key for your provider: GOOGLE_API_KEY (gemini), OPENAI_API_KEY (openai), ANTHROPIC_API_KEY (anthropic).`,
	Args: cobra.ExactArgs(1),
	RunE: runQuery,
}

func init() {
	rootCmd.PersistentFlags().StringVar(
		&provider,
		"provider",
		"gemini",
		"LLM provider: gemini, openai, or anthropic",
	)
	rootCmd.PersistentFlags().StringVar(
		&model,
		"model",
		"",
		"Model name (default: gemini-2.5-flash for gemini, gpt-4o for openai, claude-sonnet-4-20250514 for anthropic)",
	)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
