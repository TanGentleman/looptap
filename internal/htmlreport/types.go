package htmlreport

import "fmt"

// BranchMode selects how Resolve picks a branch from the target repo.
type BranchMode string

const (
	BranchCurrent BranchMode = "current" // whatever HEAD points at
	BranchDefault BranchMode = "default" // repo's default branch (origin/HEAD or main/master)
	BranchCustom  BranchMode = "custom"  // use BranchName verbatim
)

// Agent selects which coding-agent CLI we shell out to.
type Agent string

const (
	AgentClaude   Agent = "claude"   // `claude -p` (Claude Code)
	AgentOpencode Agent = "opencode" // `opencode run` (sst/opencode)
)

// HTMLSettings is the user-facing knob bag for the html subcommand.
// Keep it small until the AI layer actually needs more — model name, tone,
// section toggles, etc. will land here as they earn their keep.
type HTMLSettings struct {
	RepoPath           string     // path to a git repo; "" means cwd
	BranchMode         BranchMode // current | default | custom
	BranchName         string     // only read when BranchMode == BranchCustom
	Agent              Agent      // claude | opencode; "" defaults to claude
	OpencodeConfigPath string     // path to opencode JSON config; required when Agent == AgentOpencode
	IsSandbox          bool       // opt into permissive defaults + --dangerously-skip-permissions (opencode only)
}

// Resolved is HTMLSettings after we've poked the filesystem and asked git
// what's really there. Everything downstream works off this.
type Resolved struct {
	RepoPath           string // absolute path, confirmed to be a git repo
	BranchMode         BranchMode
	Branch             string // concrete branch name
	Agent              Agent  // concrete agent (claude or opencode)
	OpencodeConfigPath string // absolute path to opencode config, or "" for claude
	IsSandbox          bool   // see HTMLSettings.IsSandbox
}

// Summary is the short blurb we print for the confirmation prompt.
func (r *Resolved) Summary() string {
	s := fmt.Sprintf("repo:   %s\nbranch: %s (%s)\nagent:  %s", r.RepoPath, r.Branch, r.BranchMode, r.Agent)
	if r.Agent == AgentOpencode {
		cfg := r.OpencodeConfigPath
		if cfg == "" {
			if r.IsSandbox {
				cfg = "(built-in sandbox default — bash: allow)"
			} else {
				cfg = "(built-in default — bash: narrow git allowlist)"
			}
		}
		s += fmt.Sprintf("\nconfig: %s", cfg)
		if r.IsSandbox {
			s += "\nsandbox: yes (--dangerously-skip-permissions on)"
		}
	}
	return s
}
