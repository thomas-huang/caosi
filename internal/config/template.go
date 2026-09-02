package config

// SampleProviderFile is written on first run. Comments and trailing commas are
// intentional JSONC.
const SampleProviderFile = `// caosi 的 Provider 文件。
//
// 对象的 key 就是 Provider Name，也是 URL 第一段：
//   http://127.0.0.1:9999/{provider_name}/...
//
// protocol 只能是下面四个：
//   openai_chat | openai_responses | claude_messages | gemini
//
// 填好 api_key 和 base_url 之后，再运行 caosi。
{
  "deepseek": {
    "base_url": "https://api.deepseek.com",
    "protocol": "openai_chat",
    "api_key": "sk-your-key-here",
    // Claude Code 会传来 claude-* 模型名；上游是 OpenAI 兼容时请改成上游认识的名字
    "model": "deepseek-chat",
    // 可选：额外请求头（覆盖同名）
    // "headers": {
    //   "HTTP-Referer": "https://localhost",
    // },
  },
}
`
