package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/josephpaul/opsagent/internal/config"
	"github.com/josephpaul/opsagent/internal/storage"
	"github.com/spf13/cobra"
)

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear conversation history",
	Long: `Remove stored conversation history so the next query starts fresh.

By default clears the CLI session. Use --user to clear a specific user's history
(e.g. a Telegram chat).

  opsagent clear                Clear CLI conversation history
  opsagent clear --user tg-123  Clear a specific Telegram chat's history
  opsagent clear --all          Clear all stored sessions`,
	RunE: runClear,
}

var (
	clearUser string
	clearAll  bool
)

func init() {
	clearCmd.Flags().StringVar(&clearUser, "user", "", "Clear history for a specific user ID")
	clearCmd.Flags().BoolVar(&clearAll, "all", false, "Clear all stored sessions")
	rootCmd.AddCommand(clearCmd)
}

func runClear(cmd *cobra.Command, args []string) error {
	dir, err := config.Dir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}
	dbPath := filepath.Join(dir, "sessions.db")

	store, err := storage.NewSQLiteService(dbPath)
	if err != nil {
		return fmt.Errorf("open session store: %w", err)
	}
	defer store.Close()

	ctx := context.Background()

	if clearAll {
		if err := store.DeleteAllSessions(ctx); err != nil {
			return fmt.Errorf("clear all sessions: %w", err)
		}
		fmt.Println("All conversation history cleared.")
		return nil
	}

	userID := "cli"
	if clearUser != "" {
		userID = clearUser
	}

	if err := store.DeleteUserSessions(ctx, "opsagent", userID); err != nil {
		return fmt.Errorf("clear history: %w", err)
	}

	fmt.Printf("Conversation history cleared for user %q.\n", userID)
	return nil
}
