package cmd

import (
	"bufio"
	"fmt"
	"io"
	"looptap/internal/htmlreport"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func NewHTMLCmd() *cobra.Command {
	return newHTMLCmd(nil)
}

// newHTMLCmd lets tests inject a fake runner so they don't need to shell out
// to a real agent binary. Production code calls NewHTMLCmd, which passes nil
// and lets htmlreport.Generate fall back to the real CLI on PATH.
func newHTMLCmd(runner htmlreport.Runner) *cobra.Command {
	var (
		repoPath           string
		branchFlag         string
		outputPath         string
		agentFlag          string
		opencodeConfigPath string
		isSandbox          bool
		force              bool
	)

	cmd := &cobra.Command{
		Use:   "html",
		Short: "Generate an HTML branch report for the dev team",
		Long: `Analyzes a git branch and writes a self-contained HTML page that
communicates the branch narrative to the rest of the team.

Under the hood this runs a coding-agent CLI in headless mode inside the target
repo, with read-only tools so it can poke at git. Two agents are supported:

  claude    ` + "`claude -p`" + ` with --permission-mode bypassPermissions (default)
  opencode  ` + "`opencode run`" + `. Without --opencode-config, ships an embedded
            default that's safe to run against an untrusted repo on your
            laptop: read/glob/grep/list + a narrow git-read-only bash
            allowlist, edit/webfetch/websearch denied. --is-sandbox swaps
            that for the permissive shape (bash: allow, --dangerously-skip-
            permissions on) — use it in CI or disposable containers where
            the blast radius is already contained. Template:
            internal/htmlreport/opencode.default.json.

Repo, branch, agent, and sandbox may be set via flags or environment variables:
  LOOPTAP_REPO_PATH         path to a git repo (default: cwd)
  LOOPTAP_BRANCH            current | default | <branch-name> (default: current)
  LOOPTAP_AGENT             claude | opencode (default: claude)
  LOOPTAP_OPENCODE_CONFIG   path to opencode JSON config (default: built-in)
  LOOPTAP_SANDBOX           1/true to enable --is-sandbox (default: off)
  LOOPTAP_CLAUDE_BIN        override the claude binary (default: claude on PATH)
  LOOPTAP_OPENCODE_BIN      override the opencode binary (default: opencode on PATH)

Use --force to skip the confirmation prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// flag > env > default
			if repoPath == "" {
				repoPath = os.Getenv("LOOPTAP_REPO_PATH")
			}
			if branchFlag == "" {
				branchFlag = os.Getenv("LOOPTAP_BRANCH")
			}
			if agentFlag == "" {
				agentFlag = os.Getenv("LOOPTAP_AGENT")
			}
			if opencodeConfigPath == "" {
				opencodeConfigPath = os.Getenv("LOOPTAP_OPENCODE_CONFIG")
			}
			if !isSandbox {
				isSandbox = parseBool(os.Getenv("LOOPTAP_SANDBOX"))
			}

			mode, name := htmlreport.ParseBranchFlag(branchFlag)
			agent := htmlreport.ParseAgentFlag(agentFlag)
			resolved, err := htmlreport.Resolve(htmlreport.HTMLSettings{
				RepoPath:           repoPath,
				BranchMode:         mode,
				BranchName:         name,
				Agent:              agent,
				OpencodeConfigPath: opencodeConfigPath,
				IsSandbox:          isSandbox,
			})
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "looptap html — branch report")
			fmt.Fprintln(out, resolved.Summary())

			if force {
				fmt.Fprintln(out, "(--force: skipping confirmation)")
			} else {
				ok, err := confirm(cmd.InOrStdin(), out, "Proceed? [y/N]: ")
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "Aborted.")
					return nil
				}
			}

			fmt.Fprintf(out, "Asking %s... (this can take a minute)\n", resolved.Agent)
			html, err := htmlreport.Generate(cmd.Context(), resolved, runner)
			if err != nil {
				return fmt.Errorf("generating HTML: %w", err)
			}

			if outputPath == "" {
				fmt.Fprintln(out, html)
				return nil
			}
			if err := os.WriteFile(outputPath, []byte(html), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", outputPath, err)
			}
			fmt.Fprintf(out, "wrote %s\n", outputPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&repoPath, "repo", "", "path to a git repo (default: cwd, env LOOPTAP_REPO_PATH)")
	cmd.Flags().StringVar(&branchFlag, "branch", "", "current | default | <branch-name> (default: current, env LOOPTAP_BRANCH)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "write HTML to file (default: stdout)")
	cmd.Flags().StringVar(&agentFlag, "agent", "", "claude | opencode (default: claude, env LOOPTAP_AGENT)")
	cmd.Flags().StringVar(&opencodeConfigPath, "opencode-config", "", "path to opencode JSON config (default: built-in read-only config, env LOOPTAP_OPENCODE_CONFIG)")
	cmd.Flags().BoolVar(&isSandbox, "is-sandbox", false, "opencode only: opt into the permissive default (bash: allow) + --dangerously-skip-permissions (env LOOPTAP_SANDBOX)")
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")

	return cmd
}

// parseBool accepts the usual 1/true/yes/on (any case). Anything else is false.
func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// confirm reads a y/n answer from in and returns true only for an explicit yes.
// Anything else (including a bare newline) counts as no — the prompt default.
func confirm(in io.Reader, out io.Writer, prompt string) (bool, error) {
	fmt.Fprint(out, prompt)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("reading confirmation: %w", err)
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}
