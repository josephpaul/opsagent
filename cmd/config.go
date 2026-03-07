package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/josephpaul/opsagent/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage API keys and settings",
	Long: `Configure opsagent entirely from the command line. Settings are stored locally.

  opsagent config set          Interactive setup
  opsagent config set-key      Set a specific key
  opsagent config set-provider Set the default provider
  opsagent config set-model    Set the default model
  opsagent config set-base-url Set the OpenAI-compatible base URL
  opsagent config unset        Remove a saved setting
  opsagent config show         Show current configuration
  opsagent config path         Show config file location`,
}

var configSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Interactive setup — choose a provider and enter your API key",
	RunE:  runConfigSet,
}

var configSetKeyCmd = &cobra.Command{
	Use:   "set-key [KEY] [VALUE]",
	Short: "Set a specific config key (e.g. OPENAI_API_KEY sk-...)",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSetKey,
}

var configSetProviderCmd = &cobra.Command{
	Use:   "set-provider [gemini|openai|anthropic]",
	Short: "Set the default provider",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigSetProvider,
}

var configSetModelCmd = &cobra.Command{
	Use:   "set-model [MODEL]",
	Short: "Set the default model to use when --model is omitted",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigSetModel,
}

var configSetBaseURLCmd = &cobra.Command{
	Use:   "set-base-url [URL]",
	Short: "Set the OpenAI-compatible base URL",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigSetBaseURL,
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset [KEY]",
	Short: "Remove a saved config key",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigUnset,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration (keys are masked)",
	RunE:  runConfigShow,
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the config file location",
	RunE:  runConfigPath,
}

func init() {
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configSetKeyCmd)
	configCmd.AddCommand(configSetProviderCmd)
	configCmd.AddCommand(configSetModelCmd)
	configCmd.AddCommand(configSetBaseURLCmd)
	configCmd.AddCommand(configUnsetCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configPathCmd)
	rootCmd.AddCommand(configCmd)
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("OpsAgent-AI Configuration")
	fmt.Println("─────────────────────────")
	fmt.Println()
	fmt.Println("Which LLM provider do you want to use?")
	fmt.Println("  1) Gemini   (requires GOOGLE_API_KEY)")
	fmt.Println("  2) OpenAI   (requires OPENAI_API_KEY)")
	fmt.Println("  3) Anthropic (requires ANTHROPIC_API_KEY)")
	fmt.Println()
	fmt.Print("Enter choice [1/2/3]: ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	var envKey, providerName, keyHint string
	switch choice {
	case "1", "gemini":
		envKey = "GOOGLE_API_KEY"
		providerName = "gemini"
		keyHint = "Get one at https://aistudio.google.com/app/apikey"
	case "2", "openai":
		envKey = "OPENAI_API_KEY"
		providerName = "openai"
		keyHint = "Get one at https://platform.openai.com/api-keys"
	case "3", "anthropic":
		envKey = "ANTHROPIC_API_KEY"
		providerName = "anthropic"
		keyHint = "Get one at https://console.anthropic.com/"
	default:
		return fmt.Errorf("invalid choice: %q (enter 1, 2, or 3)", choice)
	}

	fmt.Println()
	fmt.Printf("  %s\n", keyHint)
	fmt.Printf("Enter your %s: ", envKey)

	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	if err := config.Set(envKey, apiKey); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	// Also save the default provider so auto-detect picks it up.
	if err := config.Set("OPSAGENT_PROVIDER", providerName); err != nil {
		return fmt.Errorf("save provider: %w", err)
	}

	path, _ := config.FilePath()
	fmt.Println()
	fmt.Printf("Saved to %s\n", path)
	fmt.Println()
	fmt.Println("You're all set! Try:")
	fmt.Printf("  opsagent \"check cpu\"\n")
	return nil
}

func runConfigSetProvider(cmd *cobra.Command, args []string) error {
	providerName := strings.ToLower(strings.TrimSpace(args[0]))
	switch providerName {
	case "gemini", "openai", "anthropic":
	default:
		return fmt.Errorf("invalid provider %q (use gemini, openai, or anthropic)", providerName)
	}
	if err := config.Set("OPSAGENT_PROVIDER", providerName); err != nil {
		return fmt.Errorf("save provider: %w", err)
	}
	path, _ := config.FilePath()
	fmt.Printf("Set default provider to %s in %s\n", providerName, path)
	return nil
}

func runConfigSetModel(cmd *cobra.Command, args []string) error {
	modelName := strings.TrimSpace(args[0])
	if modelName == "" {
		return fmt.Errorf("model cannot be empty")
	}
	if err := config.Set("OPSAGENT_MODEL", modelName); err != nil {
		return fmt.Errorf("save model: %w", err)
	}
	path, _ := config.FilePath()
	fmt.Printf("Set default model to %s in %s\n", modelName, path)
	return nil
}

func runConfigSetBaseURL(cmd *cobra.Command, args []string) error {
	baseURL := strings.TrimSpace(args[0])
	if baseURL == "" {
		return fmt.Errorf("base URL cannot be empty")
	}
	if err := config.Set("OPENAI_BASE_URL", baseURL); err != nil {
		return fmt.Errorf("save base URL: %w", err)
	}
	path, _ := config.FilePath()
	fmt.Printf("Set OPENAI_BASE_URL in %s\n", path)
	return nil
}

func runConfigSetKey(cmd *cobra.Command, args []string) error {
	key := strings.TrimSpace(args[0])
	value := strings.TrimSpace(args[1])
	if err := config.Set(key, value); err != nil {
		return fmt.Errorf("save: %w", err)
	}
	path, _ := config.FilePath()
	fmt.Printf("Set %s in %s\n", key, path)
	return nil
}

func runConfigUnset(cmd *cobra.Command, args []string) error {
	key := strings.TrimSpace(args[0])
	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}
	if err := config.Delete(key); err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	path, _ := config.FilePath()
	fmt.Printf("Removed %s from %s\n", key, path)
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	pairs, err := config.Read()
	if err != nil {
		return err
	}
	path, _ := config.FilePath()
	fmt.Printf("Config file: %s\n\n", path)
	if len(pairs) == 0 {
		fmt.Println("  (empty — run 'opsagent config set' to configure)")
		return nil
	}
	for k, v := range pairs {
		fmt.Printf("  %s: %s\n", k, maskValue(k, v))
	}
	fmt.Println()

	activeProvider := ""

	// Show which provider would be used.
	if preferred := strings.TrimSpace(pairs["OPSAGENT_PROVIDER"]); preferred != "" {
		switch preferred {
		case "gemini":
			if pairs["GOOGLE_API_KEY"] != "" {
				activeProvider = preferred + " (from OPSAGENT_PROVIDER)"
			}
		case "openai":
			if pairs["OPENAI_API_KEY"] != "" {
				activeProvider = preferred + " (from OPSAGENT_PROVIDER)"
			}
		case "anthropic":
			if pairs["ANTHROPIC_API_KEY"] != "" {
				activeProvider = preferred + " (from OPSAGENT_PROVIDER)"
			}
		}
	}
	if activeProvider == "" {
		for _, p := range []struct {
			name, key string
		}{
			{"gemini", "GOOGLE_API_KEY"},
			{"openai", "OPENAI_API_KEY"},
			{"anthropic", "ANTHROPIC_API_KEY"},
		} {
			if v := pairs[p.key]; v != "" {
				activeProvider = p.name + " (from " + p.key + ")"
				break
			}
		}
	}
	if activeProvider != "" {
		fmt.Printf("Active provider: %s\n", activeProvider)
	}
	if modelName := strings.TrimSpace(pairs["OPSAGENT_MODEL"]); modelName != "" {
		fmt.Printf("Default model: %s\n", modelName)
	}
	return nil
}

func runConfigPath(cmd *cobra.Command, args []string) error {
	path, err := config.FilePath()
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func maskValue(key, value string) string {
	key = strings.ToUpper(key)
	if strings.Contains(key, "KEY") || strings.Contains(key, "SECRET") || strings.Contains(key, "TOKEN") {
		if len(value) <= 8 {
			return "****"
		}
		return value[:4] + "..." + value[len(value)-4:]
	}
	return value
}
