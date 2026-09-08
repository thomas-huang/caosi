<p align="center">
  <img src="assets/logo/caosi-icon.svg" width="88" height="88" alt="caosi">
</p>

# caosi

[English](README.md)

不换 Claude Code、Codex 或 Gemini CLI。把它们指到你自己的上游，不必换客户端。

把 Base URL 设成 `http://127.0.0.1:9999/{provider_name}`。OpenAI Chat、OpenAI Responses、Claude Messages、Gemini 任意两两互转；协议相同就直通。

- 一个静态二进制。不需要 Docker。
- npm 包自带各平台二进制：安装不跑脚本，也不访问 GitHub
- macOS、Linux、Windows

```bash
npm install -g @thomas-huang/caosi
```

没有 Web UI，没有 OAuth，没有密钥池，也不会改写客户端配置。一个进程，一个文件，只监听本机。

## 安装

```bash
npm install -g @thomas-huang/caosi
caosi --version
```

需要 Node.js 18+，支持 macOS、Linux（amd64 和 arm64）、Windows amd64。npm 包自带各平台二进制，安装时没有脚本，也不会访问 GitHub。

## 运行

第一次启动会写入一份 Provider 样例然后退出。这是故意的：空配置不会开始监听。

```bash
caosi
```

编辑 `~/.caosi/providers.jsonc`，填入 `api_key` 和 `base_url`。若 Claude Code 要打 OpenAI 兼容上游，请保留 `model`（例如 `qwen/qwen3-coder`），否则上游会看到 `claude-*`。

再启动：

```bash
caosi
curl -s http://127.0.0.1:9999/health
```

## 把 Claude Code 指过来

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:9999/openrouter
export ANTHROPIC_API_KEY=dummy
claude
```

把 `openrouter` 换成你的 Provider Name。caosi 不用客户端带来的密钥，只用 Provider 里的 `api_key`。Claude Code 会打 `/v1/messages`；上游不是 Claude 时会转换。

## Codex / OpenAI 与 Gemini CLI

OpenAI SDK、Chat Completions、Codex：

```bash
export OPENAI_BASE_URL=http://127.0.0.1:9999/openrouter/v1
export OPENAI_API_KEY=dummy
```

Gemini CLI（`~/.gemini/.env` 或环境变量）：

```bash
export GEMINI_API_BASE=http://127.0.0.1:9999/openrouter
```

## Provider 文件

`~/.caosi/providers.jsonc` 是 JSONC，key 就是 Provider Name（URL 第一段）。`health` 是保留名。

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

| `protocol` | 上游协议 |
|---|---|
| `openai_chat` | OpenAI Chat Completions |
| `openai_responses` | OpenAI Responses |
| `claude_messages` | Claude Messages |
| `gemini` | Gemini generateContent |

- `model` 可选：填了就改写客户端带来的模型名，协议相同（直通）时也一样。
- `headers` 可选，用来补 OpenRouter 之类的额外头。
- `base_url` 是前缀。caosi 不会剥 `/v1`。写成上游真正要接的根（`https://openrouter.ai/api`，不要写成 `https://openrouter.ai/api/v1`）。

协议相同就原样转发。协议不同会改写请求体：文本、系统指令、工具、思考、Thinking Signature，以及消息里的图片、文档、音频、视频，还有 Usage Details（reasoning / cache-read / cache-create 计数），对端能表达就保留；不能表达的部分丢掉，其余照发。不会去拉取 URL，也不会走 Files API（`file_id`、`fileUri`）。独立的图片 / 文件 / 音频 / 视频 / embeddings 路径仍然是 404。

## 监听、健康检查、热加载

- 只绑 loopback：`127.0.0.1`（或用 `--listen ::1`）。默认端口 `9999`（`--port`）。
- `GET /health` 返回当前 Provider Name 列表。
- 保存 `providers.jsonc` 会热加载；坏文件保留上一份可用配置。
- 请求体和 JSON 响应超过 32MiB 会 413。
- 选项：`--config-dir`、`--port`、`--listen`、`--log-level`、`--version`。

## 明确不做

caosi 是 Converter，不是网关。

没有 Web UI、OAuth、故障转移、密钥池，也不会改你的 Claude Code / Codex / Gemini 配置文件。

## 没有 Node.js 时

从 [GitHub Releases](https://github.com/thomas-huang/caosi/releases) 下载对应系统的二进制，改名为 `caosi`（Windows 为 `caosi.exe`），放到 `PATH`。校验和在每份 Release 的 `checksums.txt`。

## 贡献者

```bash
go install github.com/thomas-huang/caosi/cmd/caosi@latest
go test ./...
```

领域用语：[CONTEXT.md](CONTEXT.md)。架构：[docs/architecture.md](docs/architecture.md)。决定：[docs/adr](docs/adr)。

发版：推 tag `vX.Y.Z`。GitHub Actions 会编五个二进制、写 checksum、建 Release，并用 GitHub OIDC Trusted Publishing 把 `@thomas-huang/caosi` 发到 npm（不用 access token）。

在 npmjs.com 打开包 `@thomas-huang/caosi` → Trusted Publisher，必须和下面**完全一致**：

- Publisher：GitHub Actions
- Organization or user：`thomas-huang`
- Repository：`caosi`（不要写成 `thomas-huang/caosi`）
- Workflow filename：`release.yml`（只要文件名，不要填 `release`，也不要填 `.github/workflows/release.yml`）
- Environment：留空（workflow 没有 `environment:`）

`OIDC permission denied for this action` 表示 OIDC 令牌已经签发，但上面几项和这次 Actions 对不上。Provenance 仍可能成功；npm 拦的是 PUT 包。
