package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const Version = "1.0.0"

var provider string
var model string
var versionFlag bool

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	RunE:  runVersion,
}

var rootCmd = &cobra.Command{
	Use:           "opsagent [query]",
	Short:         "Diagnose this machine using natural language",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `opsagent is a CLI that diagnoses the current machine using an AI agent and local diagnostics.

Example usage:
  opsagent "check cpu"
  opsagent "why is my server slow"
  opsagent --provider openai --model gpt-4o "check disk usage"

First-time setup:
  opsagent config set

Manage settings:
  opsagent config show
  opsagent config set-provider openai
  opsagent config set-model gpt-4o`,
	Args: func(cmd *cobra.Command, args []string) error {
		if versionFlag {
			return nil
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	RunE: runQuery,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if versionFlag {
			fmt.Printf("opsagent %s\n", Version)
			os.Exit(0)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.PersistentFlags().BoolVarP(&versionFlag, "version", "v", false, "Print version and exit")
	rootCmd.PersistentFlags().StringVar(
		&provider,
		"provider",
		"",
		"LLM provider: gemini, openai, or anthropic (default: auto-detect from configured keys)",
	)
	rootCmd.PersistentFlags().StringVar(
		&model,
		"model",
		"",
		"Model name (default: gemini-2.5-flash for gemini, gpt-4o for openai, claude-sonnet-4-20250514 for anthropic)",
	)
}

func runVersion(cmd *cobra.Command, args []string) error {
	fmt.Printf("opsagent %s\n", Version)
	return nil
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
