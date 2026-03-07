// Package main provides the opsagent-ai CLI for diagnosing the current machine via natural language.
package main

import (
	"os"

	"github.com/joho/godotenv"
	"github.com/josephpaul/opsagent-ai/cmd"
)

func main() {
	// Load .env so API keys can be set in a file (optional; env vars still override).
	envPath := os.Getenv("OPSAGENT_ENV")
	if envPath == "" {
		envPath = ".env"
	}
	_ = godotenv.Load(envPath)

	cmd.Execute()
}
