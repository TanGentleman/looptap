package cmd

import (
	"fmt"
	"looptap/internal/claudeenv"

	"github.com/spf13/cobra"
)

func NewEnvCmd() *cobra.Command {
	return newEnvCmd(claudeenv.Input{})
}

// newEnvCmd lets tests pin the filesystem and the inherited environment so
// the command can be exercised without reading the real home directory.
func newEnvCmd(base claudeenv.Input) *cobra.Command {
	var dir string

	cmd := &cobra.Command{
		Use:   "env",
		Short: "Print the environment variables Claude's Bash tool can read",
		Long: `Print the environment a Claude Code Bash command would see in this
directory — shell variables, overlaid with the env Claude applies from its
own config, which is the set the model can print.

Sources, later ones winning:

  ~/.claude.json                         global config env (or $CLAUDE_CONFIG_DIR/.claude.json)
  ~/.claude/settings.json                user settings
  <dir>/.claude/settings.json            project settings
  <dir>/.claude/settings.local.json      local settings left in the launch directory
  <git-root>/.claude/settings.local.json local settings at the main checkout
  <managed>/managed-settings.json        managed settings, then managed-settings.d/*.json

On Linux the managed directory is /etc/claude-code. When
CLAUDE_CODE_SUBPROCESS_ENV_SCRUB is on, credential names Claude strips
before spawning Bash are omitted. Output is KEY=value, sorted by name.

Session-only variables (the /env command, SessionStart hook scripts) exist
inside a live Claude session and are not included.`,
		RunE: func(c *cobra.Command, args []string) error {
			in := base
			if dir != "" {
				in.Dir = dir
			}
			res, err := claudeenv.Resolve(in)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			for _, line := range res.Lines {
				fmt.Fprintln(out, line)
			}
			if len(res.Warnings) > 0 {
				errOut := c.ErrOrStderr()
				for _, w := range res.Warnings {
					fmt.Fprintln(errOut, w)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "directory Claude was launched in (default: current directory)")
	return cmd
}
