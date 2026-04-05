package main

import (
	"fmt"
	"looptap/cmd"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var dbPath string

	rootCmd := &cobra.Command{
		Use:   "looptap",
		Short: "Parse agent transcripts, compute signals, write to SQLite",
		Long:  "looptap parses local coding agent transcripts (Claude Code, Codex, etc.), computes lightweight behavioral signals, and writes everything to a SQLite database.",
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "database path (default ~/.looptap/looptap.db)")

	rootCmd.AddCommand(
		cmd.NewParseCmd(&dbPath),
		cmd.NewSignalCmd(&dbPath),
		cmd.NewRunCmd(&dbPath),
		cmd.NewInfoCmd(&dbPath),
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
