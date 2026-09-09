package convert

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestRequest_ClaudeToChat_TextAndSystem(t *testing.T) {
	in := []byte(`{
	  "model": "claude-opus",
	  "max_tokens": 128,
	  "system": "be brief",
	  "messages": [{"role":"user","content":"hi"}]
	}`)
	out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "deepseek-chat", false)
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

func TestRequest_ClaudeToChat_Tools(t *testing.T) {
	in := []byte(`{
	  "model": "claude-opus",
	  "max_tokens": 32,
	  "messages": [{"role":"user","content":"weather"}],
	  "tools": [{"name":"get_weather","description":"w","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`)
	out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "", false)
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

func TestResponse_ChatToClaude_Text(t *testing.T) {
	in := []byte(`{
	  "id": "chatcmpl-1",
	  "model": "deepseek-chat",
	  "choices": [{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
	  "usage": {"prompt_tokens": 3, "completion_tokens": 1}
	}`)
	out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in)
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

func TestResponse_ChatToClaude_ErrorBody(t *testing.T) {
	in := []byte(`{"error":{"message":"quota","type":"insufficient_quota"}}`)
	out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in)
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

func TestResponse_GatewayCodeMessage_IsClientError(t *testing.T) {
	// Live Claude/gateway 400: top-level code+message, no "error" object.
	in := []byte(`{"request_id":"9aaacc31-0f6d-425b-818a-98b63c3ae714","code":"InvalidParameter","message":"The file format is illegal and cannot be opened"}`)
	cases := []struct {
		client config.Protocol
		want   []string
		leak   []string
	}{
		{
			client: config.ProtocolOpenAIChat,
			want:   []string{`"error"`, "The file format is illegal and cannot be opened"},
			leak:   []string{`"choices"`, `"chatcmpl_caosi"`},
		},
		{
			client: config.ProtocolOpenAIResponses,
			want:   []string{`"error"`, "The file format is illegal and cannot be opened"},
			leak:   []string{`"output"`, `"resp_caosi"`, `"status":"completed"`},
		},
		{
			client: config.ProtocolGemini,
			want:   []string{`"error"`, "The file format is illegal and cannot be opened", `"status"`},
			leak:   []string{`"candidates"`},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.client), func(t *testing.T) {
			out, err := Response(tc.client, config.ProtocolClaudeMessages, in)
			if err != nil {
				t.Fatal(err)
			}
			s := string(out)
			for _, w := range tc.want {
				if !strings.Contains(s, w) {
					t.Fatalf("missing %q in %s", w, s)
				}
			}
			for _, leak := range tc.leak {
				if strings.Contains(s, leak) {
					t.Fatalf("leaked %q in %s", leak, s)
				}
			}
		})
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

func TestRequest_ClaudeToChat_ToolResult(t *testing.T) {
	in := []byte(`{
	  "model": "claude-opus",
	  "max_tokens": 32,
	  "messages": [
	    {"role":"user","content":"hi"},
	    {"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"x"}}]},
	    {"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}
	  ]
	}`)
	out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "", false)
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

func TestResponse_ChatToClaude_ThinkingField(t *testing.T) {
	in := []byte(`{"choices":[{"message":{"role":"assistant","content":"hi","reasoning_content":"hmm"},"finish_reason":"stop"}]}`)
	out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in)
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

func TestGridSupported(t *testing.T) {
	protos := []config.Protocol{
		config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses,
		config.ProtocolClaudeMessages, config.ProtocolGemini,
	}
	for _, c := range protos {
		for _, u := range protos {
			if !Supported(c, u) {
				t.Fatalf("unsupported %s → %s", c, u)
			}
		}
	}
}

func TestResponsesToChatRequest(t *testing.T) {
	in := []byte(`{"model":"gpt-x","instructions":"sys","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}],"reasoning":{"effort":"high"}}`)
	out, err := Request(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, in, "deepseek-chat", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"role":"system"`) || !strings.Contains(s, "hi") {
		t.Fatalf("chat remap: %s", s)
	}
	if !strings.Contains(s, `"reasoning_effort":"high"`) {
		t.Fatalf("thinking: %s", s)
	}
	if !strings.Contains(s, "deepseek-chat") {
		t.Fatalf("model: %s", s)
	}
}

func TestClaudeToResponsesRequest(t *testing.T) {
	in := []byte(`{"model":"claude-opus","max_tokens":32,"system":"be brief","messages":[{"role":"user","content":"hi"}],"thinking":{"type":"enabled","budget_tokens":8000},"tools":[{"name":"get_weather","description":"w","input_schema":{"type":"object"}}]}`)
	out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, in, "o4-mini", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, `"choices"`) {
		t.Fatalf("leaked chat schema: %s", s)
	}
	if !strings.Contains(s, `"instructions"`) && !strings.Contains(s, "be brief") {
		t.Fatalf("system: %s", s)
	}
	if !strings.Contains(s, "get_weather") {
		t.Fatalf("tools: %s", s)
	}
	if !strings.Contains(s, `"effort"`) {
		t.Fatalf("thinking: %s", s)
	}
}

func TestGeminiRequest_ThoughtPartsPreserved(t *testing.T) {
	in := []byte(`{"contents":[{"role":"user","parts":[{"text":"hi"}]},{"role":"model","parts":[{"thought":true,"text":"secret think"},{"text":"hello"}]}]}`)
	chat, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if !strings.Contains(s, `"reasoning_content"`) || !strings.Contains(s, "secret think") {
		t.Fatalf("Gemini thought dropped on Chat Conversion:\n%s", s)
	}
	if !strings.Contains(s, "hello") {
		t.Fatalf("text dropped:\n%s", s)
	}

	claude, err := Request(config.ProtocolGemini, config.ProtocolClaudeMessages, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	cs := string(claude)
	if !strings.Contains(cs, `"type":"thinking"`) || !strings.Contains(cs, "secret think") {
		t.Fatalf("Gemini thought dropped on Claude Conversion:\n%s", cs)
	}

	resp, err := Request(config.ProtocolGemini, config.ProtocolOpenAIResponses, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	rs := string(resp)
	if !strings.Contains(rs, `"type":"reasoning"`) || !strings.Contains(rs, "secret think") {
		t.Fatalf("Gemini thought dropped on Responses Conversion:\n%s", rs)
	}
}

func TestGeminiToChatRequest(t *testing.T) {
	in := []byte(`{"systemInstruction":{"parts":[{"text":"sys"}]},"contents":[{"role":"user","parts":[{"text":"hello"},{"inlineData":{"mimeType":"image/png","data":"QQ=="}}]}],"generationConfig":{"thinkingConfig":{"thinkingLevel":"HIGH"}}}`)
	out, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, in, "deepseek-chat", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "hello") || !strings.Contains(s, `"role":"system"`) {
		t.Fatalf("gemini→chat: %s", s)
	}
	if !strings.Contains(s, "image_url") || !strings.Contains(s, "data:image/png") {
		t.Fatalf("image: %s", s)
	}
	if !strings.Contains(s, `"reasoning_effort":"high"`) {
		t.Fatalf("thinking: %s", s)
	}
}

func TestResponsesErrorNullIsSuccess(t *testing.T) {
	in := []byte(`{"id":"resp_1","object":"response","status":"completed","error":null,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}]}`)
	out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "upstream error") {
		t.Fatalf("error:null treated as failure: %s", s)
	}
	if !strings.Contains(s, `"type":"message"`) || !strings.Contains(s, "pong") {
		t.Fatalf("want Claude text, got %s", s)
	}
}

func TestResponsesImageOnlyToChat(t *testing.T) {
	in := []byte(`{"model":"gpt-x","input":[{"role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,QQ=="}]}]}`)
	out, err := Request(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, in, "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "image_url") || !strings.Contains(s, "data:image/png;base64,QQ==") {
		t.Fatalf("image-only dropped: %s", s)
	}
}

func TestChatToResponsesResponseShape(t *testing.T) {
	in := []byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"pong","reasoning_content":"hmm"},"finish_reason":"stop"}]}`)
	out, err := Response(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"choices"`) {
		t.Fatalf("chat leaked: %s", out)
	}
	if !strings.Contains(string(out), `"object":"response"`) || !strings.Contains(string(out), "pong") {
		t.Fatalf("responses: %s", out)
	}
	if !strings.Contains(string(out), "hmm") {
		t.Fatalf("thinking: %s", out)
	}
}
