package htmlreport

import _ "embed"

// DefaultOpencodeConfig is the JSON we hand to opencode when the user hasn't
// supplied a config of their own AND hasn't opted into --is-sandbox. Safe
// enough to run against a cold-clone of an untrusted repo on your laptop:
// read/glob/grep/list allowed, edit/webfetch/websearch denied, and bash is
// locked to a tight allowlist of read-only git subcommands (no `git -c`, no
// `config`/`submodule`/`fetch`/`push`/`clone` — closes textconv / alias /
// core.sshCommand escape hatches). A prompt-injected repo can't `rm -rf ~`
// through this config, which is the whole point.
//
//go:embed opencode.default.json
var DefaultOpencodeConfig []byte

// SandboxOpencodeConfig is the permissive shape, handed to opencode when the
// caller passes --is-sandbox (LOOPTAP_SANDBOX=1). Full bash, still no edit/
// webfetch/websearch. Meant for CI runners and disposable containers where
// the blast radius is already contained — not for a dev laptop.
//
//go:embed opencode.sandbox.json
var SandboxOpencodeConfig []byte
