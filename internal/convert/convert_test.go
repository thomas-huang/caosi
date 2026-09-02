package convert

import (
	"encoding/json"
	"strings"
	"testing"

	"caosi/internal/config"
)

func TestClaudeToOpenAIChat_TextAndSystem(t *testing.T) {
	in := []byte(`{
	  "model": "claude-opus",
	  "max_tokens": 128,
	  "system": "be brief",
	  "messages": [{"role":"user","content":"hi"}]
	}`)
	out, err := ClaudeToOpenAIChat(in, "deepseek-chat", false)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["model"] != "deepseek-chat" {
		t.Fatalf("model override: %v", m["model"])
	}
	msgs := m["messages"].([]any)
	sys := msgs[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "be brief" {
		t.Fatalf("system: %#v", sys)
	}
	user := msgs[1].(map[string]any)
	if user["role"] != "user" || user["content"] != "hi" {
		t.Fatalf("user: %#v", user)
	}
	if _, ok := m["stream"]; ok && m["stream"] == true {
		t.Fatal("non-stream should not force stream true")
	}
}

func TestClaudeToOpenAIChat_Tools(t *testing.T) {
	in := []byte(`{
	  "model": "claude-opus",
	  "max_tokens": 32,
	  "messages": [{"role":"user","content":"weather"}],
	  "tools": [{"name":"get_weather","description":"w","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`)
	out, err := ClaudeToOpenAIChat(in, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"function"`) {
		t.Fatalf("tools not remapped: %s", out)
	}
	if !strings.Contains(string(out), "get_weather") {
		t.Fatalf("tool name missing: %s", out)
	}
}

func TestOpenAIChatToClaude_Text(t *testing.T) {
	in := []byte(`{
	  "id": "chatcmpl-1",
	  "model": "deepseek-chat",
	  "choices": [{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
	  "usage": {"prompt_tokens": 3, "completion_tokens": 1}
	}`)
	out, err := OpenAIChatToClaude(in)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "message" {
		t.Fatalf("type=%v", m["type"])
	}
	if m["stop_reason"] != "end_turn" {
		t.Fatalf("stop_reason=%v", m["stop_reason"])
	}
	content := m["content"].([]any)
	block := content[0].(map[string]any)
	if block["type"] != "text" || block["text"] != "hello" {
		t.Fatalf("content %#v", block)
	}
}

func TestOpenAIChatToClaude_ErrorBody(t *testing.T) {
	in := []byte(`{"error":{"message":"quota","type":"insufficient_quota"}}`)
	out, err := OpenAIChatToClaude(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"error"`) {
		t.Fatalf("want claude error, got %s", out)
	}
	if !strings.Contains(string(out), "quota") {
		t.Fatalf("message dropped: %s", out)
	}
}

func TestPassthroughApplyModel(t *testing.T) {
	in := []byte(`{"model":"gpt-4","messages":[]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, in, "deepseek-chat", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"deepseek-chat"`) {
		t.Fatalf("model not rewritten: %s", out)
	}
	if strings.Contains(string(out), `"gpt-4"`) {
		t.Fatalf("old model remains: %s", out)
	}
}

func TestClaudeToOpenAIChat_ToolResult(t *testing.T) {
	in := []byte(`{
	  "model": "claude-opus",
	  "max_tokens": 32,
	  "messages": [
	    {"role":"user","content":"hi"},
	    {"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"x"}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}
	  ]
	}`)
	out, err := ClaudeToOpenAIChat(in, "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"role":"tool"`) {
		t.Fatalf("tool_result dropped: %s", s)
	}
	if !strings.Contains(s, `"tool_call_id":"toolu_1"`) {
		t.Fatalf("tool_call_id missing: %s", s)
	}
}

func TestOpenAIChatToClaude_ThinkingField(t *testing.T) {
	in := []byte(`{"choices":[{"message":{"role":"assistant","content":"hi","reasoning_content":"hmm"},"finish_reason":"stop"}]}`)
	out, err := OpenAIChatToClaude(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"thinking":"hmm"`) {
		t.Fatalf("thinking field: %s", out)
	}
	if strings.Contains(string(out), `"type":"thinking","text"`) {
		t.Fatalf("invalid thinking text field: %s", out)
	}
}

func TestUnsupportedPair(t *testing.T) {
	if Supported(config.ProtocolGemini, config.ProtocolClaudeMessages) {
		t.Fatal("gemini→claude should be unsupported in this build")
	}
}
