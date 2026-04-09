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
	var (
		repoPath   string
		branchFlag string
		outputPath string
		force      bool
	)

	cmd := &cobra.Command{
		Use:   "html",
		Short: "Generate an HTML branch report for the dev team",
		Long: `Analyzes a git branch and writes a self-contained HTML page that
communicates the branch narrative to the rest of the team.

Repo and branch may be set via flags or environment variables:
  LOOPTAP_REPO_PATH   path to a git repo (default: cwd)
  LOOPTAP_BRANCH      current | default | <branch-name> (default: current)

Use --force to skip the confirmation prompt.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// flag > env > default
			if repoPath == "" {
				repoPath = os.Getenv("LOOPTAP_REPO_PATH")
			}
			if branchFlag == "" {
				branchFlag = os.Getenv("LOOPTAP_BRANCH")
			}

			mode, name := htmlreport.ParseBranchFlag(branchFlag)
			settings := htmlreport.HTMLSettings{
				RepoPath:   repoPath,
				BranchMode: mode,
				BranchName: name,
			}

			resolved, err := htmlreport.Resolve(settings)
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

			html, err := htmlreport.Generate(resolved)
			if err != nil {
				return err
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
	cmd.Flags().BoolVar(&force, "force", false, "skip confirmation prompt")

	return cmd
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
