# Header allowlist by Upstream Protocol

caosi always sets `Host` to the upstream host from `base_url`. It strips client credentials and hop-by-hop headers, then injects the Provider credential and `headers`. Protocol capability headers are forwarded only when they match the Upstream Protocol: `anthropic-version` / `anthropic-beta` for Claude Messages, `OpenAI-Organization` / `OpenAI-Project` for OpenAI Chat and Responses.

CLIProxyAPI's header code impersonates Claude Code / Codex OAuth clients (fingerprint scrubbing, Stainless headers, wire-order casing). That is a different job. caosi does not copy it.
