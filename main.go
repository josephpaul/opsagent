// Package main provides the opsagent CLI for diagnosing the current machine via natural language.
package main

import (
	"github.com/josephpaul/opsagent-ai/cmd"
	"github.com/josephpaul/opsagent-ai/internal/config"
)

func main() {
	_ = config.Load()
	cmd.Execute()
}
