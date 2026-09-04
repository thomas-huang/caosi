# caosi

[中文](README.zh-CN.md)

A local converter between LLM client protocols and named upstream providers. Point Claude Code, Codex, or Gemini CLI at `http://127.0.0.1:9999/{provider_name}`. Same protocol is passed through; OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini are converted when they differ.

caosi is a converter, not a gateway.

## Install

```bash
npm install -g caosi
caosi --version
```

Needs Node.js 18+ on macOS, Linux, or Windows (amd64). The npm package downloads a native binary from GitHub Releases.

## Run

The first start writes a sample Provider File and exits. That is intentional: caosi will not listen on an empty config.

```bash
caosi
```

Edit `~/.caosi/providers.jsonc`. Set `api_key` and `base_url`. If Claude Code will talk to an OpenAI-compatible upstream, keep `model` (for example `deepseek-chat`) so the upstream does not see `claude-*`.

Start again:

```bash
caosi
curl -s http://127.0.0.1:9999/health
```

## Point Claude Code at it

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:9999/deepseek
export ANTHROPIC_API_KEY=dummy
claude
```

Replace `deepseek` with your Provider Name. caosi ignores the client key and uses the Provider's `api_key`. Claude Code sends `/v1/messages`; caosi converts when the upstream is not Claude.

## Codex / OpenAI and Gemini CLI

OpenAI SDK, Chat Completions, and Codex:

```bash
export OPENAI_BASE_URL=http://127.0.0.1:9999/deepseek/v1
export OPENAI_API_KEY=dummy
```

Gemini CLI (`~/.gemini/.env` or the environment):

```bash
export GEMINI_API_BASE=http://127.0.0.1:9999/deepseek
```

## Provider File

`~/.caosi/providers.jsonc` is JSONC keyed by Provider Name (the first URL path segment). `health` is reserved.

```jsonc
{
  "deepseek": {
    "base_url": "https://api.deepseek.com",
    "protocol": "openai_chat",
    "api_key": "sk-...",
    "model": "deepseek-chat"
  }
}
```

| `protocol` | Upstream wire protocol |
|---|---|
| `openai_chat` | OpenAI Chat Completions |
| `openai_responses` | OpenAI Responses |
| `claude_messages` | Claude Messages |
| `gemini` | Gemini generateContent |

- `model` is optional: when set, it replaces the client's model name on the upstream request.
- `headers` is optional extra request headers (for example OpenRouter).
- `base_url` is a prefix. caosi does not strip `/v1`. Use the root the upstream actually expects (`https://api.deepseek.com`, not `https://api.deepseek.com/v1`).

## Listen, health, reload

- Loopback only: `127.0.0.1` (or `::1` via `--listen`). Default port `9999` (`--port`).
- `GET /health`
- Saving `providers.jsonc` hot-reloads; a bad file keeps the last good config.
- Flags: `--config-dir`, `--port`, `--listen`, `--log-level`, `--version`.

## What it does not do

No Web UI, OAuth, failover, key pools, or rewriting your Claude Code / Codex / Gemini config files.

## Without Node.js

Download the binary for your OS from [GitHub Releases](https://github.com/thomas-huang/caosi/releases), rename it to `caosi` (or `caosi.exe` on Windows), and put it on your `PATH`. Checksums are in `checksums.txt` on each release.

## Contributors

```bash
go install github.com/thomas-huang/caosi/cmd/caosi@latest
go test ./...
```

Domain language: [CONTEXT.md](CONTEXT.md). Decisions: [docs/adr](docs/adr).

To cut a release, push a tag `vX.Y.Z` (first public: `v0.1.0`). GitHub Actions builds the five binaries, writes checksums, creates the Release, and publishes `caosi` to npm with GitHub OIDC trusted publishing (no access token). On npmjs.com, add a GitHub Actions trusted publisher for package `caosi`: repository `thomas-huang/caosi`, workflow filename `release.yml`.
