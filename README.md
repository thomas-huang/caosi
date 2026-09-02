# caosi

本机 LLM 协议转换器。Claude Code、Codex、Gemini CLI 打到 `http://127.0.0.1:9999/{provider_name}`，caosi 按 `providers.jsonc` 转到上游；同协议透传，Claude Messages → OpenAI Chat 会转换。

**给试用者的说明（请打开这一页）：** [docs/guide.html](docs/guide.html)

## 运行

```bash
go build -o caosi ./cmd/caosi
./caosi
```

第一次运行会在 `~/.caosi/providers.jsonc` 写下带注释的样例，然后退出。填好 `api_key`（Claude Code 打 OpenAI 兼容上游时保留 `model`）后再运行一次。

```bash
./caosi --help
./caosi --port 9999 --config-dir ~/.caosi
curl -s http://127.0.0.1:9999/health
```

只监听 loopback。领域用语见 [CONTEXT.md](CONTEXT.md)，决定见 [docs/adr](docs/adr)。
