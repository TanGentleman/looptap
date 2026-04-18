package htmlreport

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Resolve turns user-provided HTMLSettings into a concrete Resolved: an
// absolute repo path plus a real branch name that git actually knows about.
// Errors here are user-facing — the CLI prints them straight to stderr.
func Resolve(s HTMLSettings) (*Resolved, error) {
	repo, err := resolveRepo(s.RepoPath)
	if err != nil {
		return nil, err
	}

	mode := s.BranchMode
	if mode == "" {
		mode = BranchCurrent
	}

	branch, err := resolveBranch(repo, mode, s.BranchName)
	if err != nil {
		return nil, err
	}

	agent := s.Agent
	if agent == "" {
		agent = AgentClaude
	}

	cfg, err := resolveAgentConfig(agent, s.OpencodeConfigPath)
	if err != nil {
		return nil, err
	}

	return &Resolved{
		RepoPath:           repo,
		BranchMode:         mode,
		Branch:             branch,
		Agent:              agent,
		OpencodeConfigPath: cfg,
		IsSandbox:          s.IsSandbox,
	}, nil
}

// ParseAgentFlag maps --agent / LOOPTAP_AGENT into a concrete Agent.
// Unknown values flow through so Resolve can complain with a proper error.
func ParseAgentFlag(raw string) Agent {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	switch trimmed {
	case "", "claude", "claude-code":
		return AgentClaude
	case "opencode":
		return AgentOpencode
	default:
		return Agent(trimmed)
	}
}

// resolveAgentConfig validates per-agent prerequisites. Claude needs nothing
// beyond a binary on PATH; opencode needs a JSON config file — either an
// explicit --opencode-config path, or nothing (in which case we fall back to
// the embedded DefaultOpencodeConfig, materialized to a tempfile at run time).
func resolveAgentConfig(agent Agent, path string) (string, error) {
	switch agent {
	case AgentClaude:
		return "", nil
	case AgentOpencode:
		trimmed := strings.TrimSpace(path)
		if trimmed == "" {
			// Empty means "use the built-in default config". The runner
			// materializes DefaultOpencodeConfig to a tempfile at exec time.
			return "", nil
		}
		abs, err := filepath.Abs(trimmed)
		if err != nil {
			return "", fmt.Errorf("resolving opencode config %q: %w", trimmed, err)
		}
		fi, err := os.Stat(abs)
		if os.IsNotExist(err) {
			return "", fmt.Errorf("opencode config %q does not exist", abs)
		}
		if err != nil {
			return "", fmt.Errorf("stat opencode config %q: %w", abs, err)
		}
		if fi.IsDir() {
			return "", fmt.Errorf("opencode config %q is a directory, want a JSON file", abs)
		}
		return abs, nil
	default:
		return "", fmt.Errorf("unknown agent %q (want claude or opencode)", agent)
	}
}

// ParseBranchFlag maps the --branch flag (or LOOPTAP_BRANCH env) into a
// (mode, name) pair. "current" and "default" are the magic words; anything
// else is treated as a literal branch name.
func ParseBranchFlag(raw string) (BranchMode, string) {
	trimmed := strings.TrimSpace(raw)
	switch strings.ToLower(trimmed) {
	case "", "current":
		return BranchCurrent, ""
	case "default":
		return BranchDefault, ""
	default:
		return BranchCustom, trimmed
	}
}

func resolveRepo(path string) (string, error) {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("finding cwd: %w", err)
		}
		path = cwd
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", path, err)
	}

	fi, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("repo path %q does not exist", abs)
	}
	if err != nil {
		return "", fmt.Errorf("stat %q: %w", abs, err)
	}
	if !fi.IsDir() {
		return "", fmt.Errorf("repo path %q is not a directory", abs)
	}

	// Ask git for the actual toplevel — handles subdirectories and worktrees.
	out, err := runGit(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("%q is not a git repo: %w", abs, err)
	}
	return strings.TrimSpace(out), nil
}

func resolveBranch(repo string, mode BranchMode, custom string) (string, error) {
	switch mode {
	case BranchCurrent:
		out, err := runGit(repo, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return "", fmt.Errorf("reading current branch: %w", err)
		}
		name := strings.TrimSpace(out)
		if name == "" || name == "HEAD" {
			return "", fmt.Errorf("HEAD is detached — no current branch to analyze")
		}
		return name, nil

	case BranchDefault:
		// Prefer origin/HEAD if it's set; otherwise fall back to the usual suspects.
		if out, err := runGit(repo, "symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
			name := strings.TrimSpace(out)
			name = strings.TrimPrefix(name, "origin/")
			if name != "" {
				return name, nil
			}
		}
		for _, candidate := range []string{"main", "master"} {
			if _, err := runGit(repo, "rev-parse", "--verify", "refs/heads/"+candidate); err == nil {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("could not determine default branch (no origin/HEAD, no main or master)")

	case BranchCustom:
		name := strings.TrimSpace(custom)
		if name == "" {
			return "", fmt.Errorf("custom branch mode requires a branch name")
		}
		if _, err := runGit(repo, "rev-parse", "--verify", "refs/heads/"+name); err != nil {
			// Try the remote-tracking branch as a friendly fallback.
			if _, err2 := runGit(repo, "rev-parse", "--verify", "refs/remotes/origin/"+name); err2 != nil {
				return "", fmt.Errorf("branch %q not found locally or on origin", name)
			}
		}
		return name, nil

	default:
		return "", fmt.Errorf("unknown branch mode %q (want current, default, or custom)", mode)
	}
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
