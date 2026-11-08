package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/TanGentleman/looptap/cmd"

	"github.com/spf13/cobra"
)

// Version is stamped at build time via -ldflags. `go install ...@vX.Y.Z`
// builds fall back to the module version.
var Version = "dev"

func version() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
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
		cmd.NewQueryCmd(&dbPath),
		cmd.NewPatternsCmd(&dbPath),
		cmd.NewSeedContractFixtureCmd(&dbPath),
		cmd.NewAdviseCmd(&dbPath),
		cmd.NewAnalyzeCmd(),
		newVersionCmd(),
	)
	return rootCmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the looptap version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintln(cmd.OutOrStdout(), version())
		},
	}
}
