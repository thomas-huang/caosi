<p align="center">
  <img src="assets/logo/caosi-icon.svg" width="88" height="88" alt="caosi">
</p>

# caosi

[中文](README.zh-CN.md)

Keep using Claude Code, Codex, or Gemini CLI. Point them at your own upstream — no client switch.

Set the Base URL to `http://127.0.0.1:9999/{provider_name}`. OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini convert in every direction; same protocol is passed through.

- A single static binary. No Docker.
- npm ships the binaries: no install script, nothing fetched from GitHub
- macOS, Linux, and Windows

```bash
npm install -g @thomas-huang/caosi
```

No Web UI, no OAuth, no key pool, no client-config rewrite. One process, one file, loopback only.

## Install

```bash
npm install -g @thomas-huang/caosi
caosi --version
```

Needs Node.js 18+ on macOS or Linux (amd64 and arm64), or Windows (amd64). The npm package includes native binaries; there is no install script and nothing is fetched from GitHub at install time.

## Run

The first start writes a sample Provider File and exits. That is intentional: caosi will not listen on an empty config.

```bash
caosi
```

Edit `~/.caosi/providers.jsonc`. Set `api_key` and `base_url`. If Claude Code will talk to an OpenAI-compatible upstream, keep `model` (for example `qwen/qwen3-coder`) so the upstream does not see `claude-*`.

Start again:

```bash
caosi
curl -s http://127.0.0.1:9999/health
```

## Point Claude Code at it

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:9999/openrouter
export ANTHROPIC_API_KEY=dummy
claude
```

Replace `openrouter` with your Provider Name. caosi ignores the client key and uses the Provider's `api_key`. Claude Code sends `/v1/messages`; caosi converts when the upstream is not Claude.

## Codex / OpenAI and Gemini CLI

OpenAI SDK, Chat Completions, and Codex:

```bash
export OPENAI_BASE_URL=http://127.0.0.1:9999/openrouter/v1
export OPENAI_API_KEY=dummy
```

Gemini CLI (`~/.gemini/.env` or the environment):

```bash
export GEMINI_API_BASE=http://127.0.0.1:9999/openrouter
```

## Provider File

`~/.caosi/providers.jsonc` is JSONC keyed by Provider Name (the first URL path segment). `health` is reserved.

```jsonc
{
  "openrouter": {
    "base_url": "https://openrouter.ai/api",
    "protocol": "openai_chat",
    "api_key": "sk-...",
    "model": "qwen/qwen3-coder"
  }
}
```

| `protocol` | Upstream wire protocol |
|---|---|
| `openai_chat` | OpenAI Chat Completions |
| `openai_responses` | OpenAI Responses |
| `claude_messages` | Claude Messages |
| `gemini` | Gemini generateContent |

- `model` is optional: when set, it replaces the client's model name on the upstream request, including when the protocols already match.
- `headers` is optional extra request headers (for example OpenRouter).
- `base_url` is a prefix. caosi does not strip `/v1`. Use the root the upstream actually expects (`https://openrouter.ai/api`, not `https://openrouter.ai/api/v1`).

Same protocol is copied through. Different protocols remap the body: text, system instruction, tools, thinking, and in-message images, documents, audio, and video are kept when the other protocol can express them; otherwise that part is dropped and the rest is sent. caosi does not fetch URLs or Files API objects (`file_id`, `fileUri`). Dedicated image, files, audio, video, and embeddings paths stay 404.

## Listen, health, reload

- Loopback only: `127.0.0.1` (or `::1` via `--listen`). Default port `9999` (`--port`).
- `GET /health` returns JSON with the current Provider Names.
- Saving `providers.jsonc` hot-reloads; a bad file keeps the last good config.
- Request and JSON-response bodies over 32MiB are rejected (413).
- Flags: `--config-dir`, `--port`, `--listen`, `--log-level`, `--version`.

## What it does not do

caosi is a converter, not a gateway.

No Web UI, OAuth, failover, key pools, or rewriting your Claude Code / Codex / Gemini config files.

## Without Node.js

Download the binary for your OS from [GitHub Releases](https://github.com/thomas-huang/caosi/releases), rename it to `caosi` (or `caosi.exe` on Windows), and put it on your `PATH`. Checksums are in `checksums.txt` on each release.

## Contributors

```bash
go install github.com/thomas-huang/caosi/cmd/caosi@latest
go test ./...
```

Domain language: [CONTEXT.md](CONTEXT.md). Architecture: [docs/architecture.md](docs/architecture.md). Decisions: [docs/adr](docs/adr).

To cut a release, push a tag `vX.Y.Z`. GitHub Actions builds the five binaries, writes checksums, creates the Release, and publishes `@thomas-huang/caosi` to npm with GitHub OIDC trusted publishing (no access token).

On npmjs.com, open package `@thomas-huang/caosi` → Trusted Publisher and set **exactly**:

- Publisher: GitHub Actions
- Organization or user: `thomas-huang`
- Repository: `caosi` (not `thomas-huang/caosi`)
- Workflow filename: `release.yml` (filename only, not `release` and not `.github/workflows/release.yml`)
- Environment: leave empty (this workflow does not set `environment:`)

`OIDC permission denied for this action` means the token was issued but those fields did not match. Provenance can still succeed; the PUT to the registry is what npm authorizes.
