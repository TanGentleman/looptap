package htmlreport

import _ "embed"

// DefaultOpencodeConfig is the JSON we hand to opencode when the user hasn't
// supplied a config of their own. Matches the schema at
// https://opencode.ai/config.json: read/glob/grep/list/bash allowed, edit and
// web access denied — the read-only shape the branch report needs. Provider
// creds live outside this file; set ANTHROPIC_API_KEY (or whatever the chosen
// provider expects) in the environment.
//
// Copy-friendly: the exact bytes also ship as
// internal/htmlreport/opencode.default.json in the repo.
//
//go:embed opencode.default.json
var DefaultOpencodeConfig []byte
