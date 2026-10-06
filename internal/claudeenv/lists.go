package claudeenv

// Snapshots of the lists Claude Code consults when deciding which settings
// env vars may override the process, and which names disappear before Bash
// starts. They drift as Claude ships; the behavior they encode does not:
// a host that owns inference routing keeps its routing vars, and a scrubbed
// subprocess does not inherit the credential names below.

// tunnelVars are stripped from settings-sourced env when
// ANTHROPIC_UNIX_SOCKET is already set, so a remote `claude ssh` session
// can't have its forwarded auth clobbered by ~/.claude.json.
var tunnelVars = map[string]bool{
	"ANTHROPIC_UNIX_SOCKET":   true,
	"ANTHROPIC_BASE_URL":      true,
	"ANTHROPIC_API_KEY":       true,
	"ANTHROPIC_AUTH_TOKEN":    true,
	"CLAUDE_CODE_OAUTH_TOKEN": true,
}

// providerManagedVars are stripped from settings-sourced env when
// CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST is truthy. VERTEX_REGION_CLAUDE_*
// is prefix-matched in providerManaged.
var providerManagedVars = map[string]bool{
	"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST":                  true,
	"CLAUDE_CODE_USE_BEDROCK":                               true,
	"CLAUDE_CODE_USE_VERTEX":                                true,
	"CLAUDE_CODE_USE_FOUNDRY":                               true,
	"ANTHROPIC_BASE_URL":                                    true,
	"ANTHROPIC_BEDROCK_BASE_URL":                            true,
	"ANTHROPIC_VERTEX_BASE_URL":                             true,
	"ANTHROPIC_FOUNDRY_BASE_URL":                            true,
	"ANTHROPIC_FOUNDRY_RESOURCE":                            true,
	"ANTHROPIC_VERTEX_PROJECT_ID":                           true,
	"CLOUD_ML_REGION":                                       true,
	"ANTHROPIC_API_KEY":                                     true,
	"ANTHROPIC_AUTH_TOKEN":                                  true,
	"CLAUDE_CODE_OAUTH_TOKEN":                               true,
	"AWS_BEARER_TOKEN_BEDROCK":                              true,
	"ANTHROPIC_FOUNDRY_API_KEY":                             true,
	"CLAUDE_CODE_SKIP_BEDROCK_AUTH":                         true,
	"CLAUDE_CODE_SKIP_VERTEX_AUTH":                          true,
	"CLAUDE_CODE_SKIP_FOUNDRY_AUTH":                         true,
	"ANTHROPIC_MODEL":                                       true,
	"ANTHROPIC_DEFAULT_HAIKU_MODEL":                         true,
	"ANTHROPIC_DEFAULT_HAIKU_MODEL_DESCRIPTION":             true,
	"ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME":                    true,
	"ANTHROPIC_DEFAULT_HAIKU_MODEL_SUPPORTED_CAPABILITIES":  true,
	"ANTHROPIC_DEFAULT_OPUS_MODEL":                          true,
	"ANTHROPIC_DEFAULT_OPUS_MODEL_DESCRIPTION":              true,
	"ANTHROPIC_DEFAULT_OPUS_MODEL_NAME":                     true,
	"ANTHROPIC_DEFAULT_OPUS_MODEL_SUPPORTED_CAPABILITIES":   true,
	"ANTHROPIC_DEFAULT_SONNET_MODEL":                        true,
	"ANTHROPIC_DEFAULT_SONNET_MODEL_DESCRIPTION":            true,
	"ANTHROPIC_DEFAULT_SONNET_MODEL_NAME":                   true,
	"ANTHROPIC_DEFAULT_SONNET_MODEL_SUPPORTED_CAPABILITIES": true,
	"ANTHROPIC_SMALL_FAST_MODEL":                            true,
	"ANTHROPIC_SMALL_FAST_MODEL_AWS_REGION":                 true,
	"CLAUDE_CODE_SUBAGENT_MODEL":                            true,
}

// subprocessScrub is removed from the child environment when
// CLAUDE_CODE_SUBPROCESS_ENV_SCRUB is truthy, along with INPUT_<name>
// duplicates GitHub Actions synthesizes from workflow inputs.
var subprocessScrub = []string{
	"ANTHROPIC_API_KEY",
	"CLAUDE_CODE_OAUTH_TOKEN",
	"ANTHROPIC_AUTH_TOKEN",
	"ANTHROPIC_FOUNDRY_API_KEY",
	"ANTHROPIC_CUSTOM_HEADERS",
	"OTEL_EXPORTER_OTLP_HEADERS",
	"OTEL_EXPORTER_OTLP_LOGS_HEADERS",
	"OTEL_EXPORTER_OTLP_METRICS_HEADERS",
	"OTEL_EXPORTER_OTLP_TRACES_HEADERS",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"AWS_BEARER_TOKEN_BEDROCK",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"AZURE_CLIENT_SECRET",
	"AZURE_CLIENT_CERTIFICATE_PATH",
	"ACTIONS_ID_TOKEN_REQUEST_TOKEN",
	"ACTIONS_ID_TOKEN_REQUEST_URL",
	"ACTIONS_RUNTIME_TOKEN",
	"ACTIONS_RUNTIME_URL",
	"ALL_INPUTS",
	"OVERRIDE_GITHUB_TOKEN",
	"DEFAULT_WORKFLOW_TOKEN",
	"SSH_SIGNING_KEY",
}
