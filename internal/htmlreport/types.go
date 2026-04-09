package htmlreport

import "fmt"

// BranchMode selects how Resolve picks a branch from the target repo.
type BranchMode string

const (
	BranchCurrent BranchMode = "current" // whatever HEAD points at
	BranchDefault BranchMode = "default" // repo's default branch (origin/HEAD or main/master)
	BranchCustom  BranchMode = "custom"  // use BranchName verbatim
)

// HTMLSettings is the user-facing knob bag for the html subcommand.
// Keep it small until the AI layer actually needs more — model name, tone,
// section toggles, etc. will land here as they earn their keep.
type HTMLSettings struct {
	RepoPath   string     // path to a git repo; "" means cwd
	BranchMode BranchMode // current | default | custom
	BranchName string     // only read when BranchMode == BranchCustom
}

// Resolved is HTMLSettings after we've poked the filesystem and asked git
// what's really there. Everything downstream works off this.
type Resolved struct {
	RepoPath   string // absolute path, confirmed to be a git repo
	BranchMode BranchMode
	Branch     string // concrete branch name
}

// Summary is the short blurb we print for the confirmation prompt.
func (r *Resolved) Summary() string {
	return fmt.Sprintf("repo:   %s\nbranch: %s (%s)", r.RepoPath, r.Branch, r.BranchMode)
}
