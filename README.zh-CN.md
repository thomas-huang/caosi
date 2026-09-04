# caosi

[English](README.md)

本机 LLM 协议转换器。把 Claude Code、Codex 或 Gemini CLI 指到 `http://127.0.0.1:9999/{provider_name}`。同协议透传；OpenAI Chat、OpenAI Responses、Claude Messages、Gemini 之间在协议不同时会转换。

caosi 是 Converter，不是网关。

## 安装

```bash
npm install -g caosi
caosi --version
```

需要 Node.js 18+，支持 macOS、Linux、Windows amd64。npm 包会从 GitHub Releases 下载对应平台的二进制。

## 运行

第一次启动会写入一份 Provider 样例然后退出。这是故意的：空配置不会开始监听。

```bash
caosi
```

编辑 `~/.caosi/providers.jsonc`，填入 `api_key` 和 `base_url`。若 Claude Code 要打 OpenAI 兼容上游，请保留 `model`（例如 `deepseek-chat`），否则上游会看到 `claude-*`。

再启动：

```bash
caosi
curl -s http://127.0.0.1:9999/health
```

## 把 Claude Code 指过来

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:9999/deepseek
export ANTHROPIC_API_KEY=dummy
claude
```

把 `deepseek` 换成你的 Provider Name。caosi 不用客户端带来的密钥，只用 Provider 里的 `api_key`。Claude Code 会打 `/v1/messages`；上游不是 Claude 时会转换。

## Codex / OpenAI 与 Gemini CLI

OpenAI SDK、Chat Completions、Codex：

```bash
export OPENAI_BASE_URL=http://127.0.0.1:9999/deepseek/v1
export OPENAI_API_KEY=dummy
```

Gemini CLI（`~/.gemini/.env` 或环境变量）：

```bash
export GEMINI_API_BASE=http://127.0.0.1:9999/deepseek
```

## Provider 文件

`~/.caosi/providers.jsonc` 是 JSONC，key 就是 Provider Name（URL 第一段）。`health` 是保留名。

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

| `protocol` | 上游协议 |
|---|---|
| `openai_chat` | OpenAI Chat Completions |
| `openai_responses` | OpenAI Responses |
| `claude_messages` | Claude Messages |
| `gemini` | Gemini generateContent |

- `model` 可选：填了就改写客户端带来的模型名。
- `headers` 可选，用来补 OpenRouter 之类的额外头。
- `base_url` 是前缀。caosi 不会剥 `/v1`。写成上游真正要接的根（`https://api.deepseek.com`，不要写成 `https://api.deepseek.com/v1`）。

## 监听、健康检查、热加载

- 只绑 loopback：`127.0.0.1`（或用 `--listen ::1`）。默认端口 `9999`（`--port`）。
- `GET /health`
- 保存 `providers.jsonc` 会热加载；坏文件保留上一份可用配置。
- 选项：`--config-dir`、`--port`、`--listen`、`--log-level`、`--version`。

## 明确不做

没有 Web UI、OAuth、故障转移、密钥池，也不会改你的 Claude Code / Codex / Gemini 配置文件。

## 没有 Node.js 时

从 [GitHub Releases](https://github.com/thomas-huang/caosi/releases) 下载对应系统的二进制，改名为 `caosi`（Windows 为 `caosi.exe`），放到 `PATH`。校验和在每份 Release 的 `checksums.txt`。

## 贡献者

```bash
go install github.com/thomas-huang/caosi/cmd/caosi@latest
go test ./...
```

领域用语：[CONTEXT.md](CONTEXT.md)。决定：[docs/adr](docs/adr)。

发版：推 tag `vX.Y.Z`（第一次公开用 `v0.1.0`）。GitHub Actions 会编五个二进制、写 checksum、建 Release，并用 GitHub OIDC Trusted Publishing 把 `caosi` 发到 npm（不用 access token）。在 npmjs.com 上为包 `caosi` 添加 GitHub Actions trusted publisher：仓库 `thomas-huang/caosi`，workflow 文件名 `release.yml`。
