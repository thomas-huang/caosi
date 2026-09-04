package convert

import (
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

var allProtocols = []config.Protocol{
	config.ProtocolOpenAIChat,
	config.ProtocolOpenAIResponses,
	config.ProtocolClaudeMessages,
	config.ProtocolGemini,
}

func representativeRequest(p config.Protocol) []byte {
	switch p {
	case config.ProtocolOpenAIChat:
		return []byte(`{
		  "model": "gpt-x",
		  "max_tokens": 32,
		  "messages": [
		    {"role": "system", "content": "be brief"},
		    {"role": "user", "content": "hi"}
		  ],
		  "tools": [{"type": "function", "function": {"name": "get_weather", "description": "w", "parameters": {"type": "object", "properties": {}}}}]
		}`)
	case config.ProtocolOpenAIResponses:
		return []byte(`{
		  "model": "gpt-x",
		  "instructions": "be brief",
		  "input": [{"role": "user", "content": [{"type": "input_text", "text": "hi"}]}],
		  "tools": [{"type": "function", "name": "get_weather", "description": "w", "parameters": {"type": "object"}}]
		}`)
	case config.ProtocolClaudeMessages:
		return []byte(`{
		  "model": "claude-opus",
		  "max_tokens": 32,
		  "system": "be brief",
		  "messages": [{"role": "user", "content": "hi"}],
		  "tools": [{"name": "get_weather", "description": "w", "input_schema": {"type": "object"}}]
		}`)
	case config.ProtocolGemini:
		return []byte(`{
		  "systemInstruction": {"parts": [{"text": "be brief"}]},
		  "contents": [{"role": "user", "parts": [{"text": "hi"}]}],
		  "tools": [{"functionDeclarations": [{"name": "get_weather", "description": "w", "parameters": {"type": "object"}}]}]
		}`)
	}
	return nil
}

func representativeResponse(p config.Protocol) []byte {
	switch p {
	case config.ProtocolOpenAIChat:
		return []byte(`{"id":"chatcmpl-1","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"pong","reasoning_content":"hmm"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	case config.ProtocolOpenAIResponses:
		return []byte(`{"id":"resp_1","object":"response","status":"completed","error":null,"output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}]}`)
	case config.ProtocolClaudeMessages:
		return []byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"pong"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	case config.ProtocolGemini:
		return []byte(`{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"hmm"},{"text":"pong"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`)
	}
	return nil
}

func TestRequest_AllSupportedPairs(t *testing.T) {
	for _, client := range allProtocols {
		for _, upstream := range allProtocols {
			client, upstream := client, upstream
			t.Run(string(client)+"→"+string(upstream), func(t *testing.T) {
				out, err := Request(client, upstream, representativeRequest(client), "m", false)
				if err != nil {
					t.Fatal(err)
				}
				s := string(out)
				if !strings.Contains(s, "hi") {
					t.Fatalf("lost user text:\n%s", s)
				}
				if !strings.Contains(s, "be brief") {
					t.Fatalf("lost system:\n%s", s)
				}
				if !strings.Contains(s, "get_weather") {
					t.Fatalf("lost tool:\n%s", s)
				}
				assertUpstreamRequestShape(t, upstream, out)
			})
		}
	}
}

func TestResponse_AllSupportedPairs(t *testing.T) {
	for _, client := range allProtocols {
		for _, upstream := range allProtocols {
			client, upstream := client, upstream
			t.Run(string(client)+"←"+string(upstream), func(t *testing.T) {
				out, err := Response(client, upstream, representativeResponse(upstream))
				if err != nil {
					t.Fatal(err)
				}
				s := string(out)
				if !strings.Contains(s, "pong") {
					t.Fatalf("lost assistant text:\n%s", s)
				}
				assertClientResponseShape(t, client, out)
			})
		}
	}
}

func TestResponse_ErrorBodiesMatchClient(t *testing.T) {
	in := []byte(`{"error":{"message":"quota","type":"insufficient_quota"}}`)
	cases := []struct {
		client config.Protocol
		want   []string
		leak   []string
	}{
		{
			client: config.ProtocolOpenAIChat,
			want:   []string{`"error"`, "quota"},
			leak:   []string{`"candidates"`, `"type":"error"`},
		},
		{
			client: config.ProtocolOpenAIResponses,
			want:   []string{`"error"`, "quota"},
			leak:   []string{`"candidates"`},
		},
		{
			client: config.ProtocolClaudeMessages,
			want:   []string{`"type":"error"`, "quota"},
			leak:   []string{`"choices"`, `"candidates"`},
		},
		{
			client: config.ProtocolGemini,
			want:   []string{`"error"`, "quota", `"status"`},
			leak:   []string{`"choices"`, `"type":"error"`},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.client), func(t *testing.T) {
			out, err := Response(tc.client, config.ProtocolOpenAIChat, in)
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

func TestRequest_ClaudeToChat_ImageBlocks(t *testing.T) {
	in := []byte(`{
	  "model": "claude-opus",
	  "max_tokens": 16,
	  "messages": [{"role":"user","content":[
	    {"type":"image","source":{"type":"base64","media_type":"image/png","data":"QQ=="}},
	    {"type":"text","text":"what"}
	  ]}]
	}`)
	out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, `"contents"`) || strings.Contains(s, `"type":"image"`) {
		t.Fatalf("Claude image schema leaked:\n%s", s)
	}
	if !strings.Contains(s, "image_url") || !strings.Contains(s, "data:image/png;base64,QQ==") {
		t.Fatalf("image dropped:\n%s", s)
	}
	if !strings.Contains(s, "what") {
		t.Fatalf("text dropped:\n%s", s)
	}
}

func TestRequest_ChatToClaude_ImageParts(t *testing.T) {
	in := []byte(`{
	  "model": "gpt-x",
	  "messages": [{"role":"user","content":[
	    {"type":"text","text":"see"},
	    {"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="}}
	  ]}]
	}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, in, "claude-opus", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, `"choices"`) || strings.Contains(s, `"contents"`) {
		t.Fatalf("leaked foreign schema:\n%s", s)
	}
	if !strings.Contains(s, `"type":"image"`) || !strings.Contains(s, "QQ==") {
		t.Fatalf("image dropped:\n%s", s)
	}
	if !strings.Contains(s, "see") {
		t.Fatalf("text dropped:\n%s", s)
	}
}

func TestRequest_ChatToGemini_ImageAndToolResult(t *testing.T) {
	in := []byte(`{
	  "model": "gpt-x",
	  "messages": [
	    {"role":"user","content":[{"type":"text","text":"see"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="}}]},
	    {"role":"tool","name":"get_weather","content":"ok"}
	  ]
	}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, in, "gemini-pro", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, `"choices"`) || strings.Contains(s, `"messages"`) {
		t.Fatalf("leaked chat schema:\n%s", s)
	}
	if !strings.Contains(s, `"contents"`) {
		t.Fatalf("want Gemini contents:\n%s", s)
	}
	if !strings.Contains(s, "inlineData") && !strings.Contains(s, "inline_data") {
		t.Fatalf("image dropped:\n%s", s)
	}
	if !strings.Contains(s, "QQ==") || !strings.Contains(s, "see") {
		t.Fatalf("parts dropped:\n%s", s)
	}
	if !strings.Contains(s, "functionResponse") && !strings.Contains(s, "function_response") {
		t.Fatalf("tool result dropped:\n%s", s)
	}
}

func TestNeedsConvertUpstreamPathAndModel(t *testing.T) {
	if NeedsConvert(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat) {
		t.Fatal("same protocol should not convert")
	}
	if !NeedsConvert(config.ProtocolOpenAIChat, config.ProtocolGemini) {
		t.Fatal("cross-protocol should convert")
	}
	if got := UpstreamPath(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, "/v1/chat/completions", "m", false); got != "/v1/chat/completions" {
		t.Fatalf("passthrough path: %s", got)
	}
	if got := UpstreamPath(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, "/v1/messages", "m", false); got != "/v1/chat/completions" {
		t.Fatalf("chat path: %s", got)
	}
	if got := UpstreamPath(config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, "/v1/chat/completions", "m", false); got != "/v1/responses" {
		t.Fatalf("responses path: %s", got)
	}
	if got := UpstreamPath(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, "/v1/chat/completions", "m", false); got != "/v1/messages" {
		t.Fatalf("claude path: %s", got)
	}
	got := UpstreamPath(config.ProtocolOpenAIChat, config.ProtocolGemini, "/v1/chat/completions", "gemini-flash", false)
	if got != "/v1beta/models/gemini-flash:generateContent" {
		t.Fatalf("gemini path: %s", got)
	}
	got = UpstreamPath(config.ProtocolOpenAIChat, config.ProtocolGemini, "/v1/chat/completions", "", true)
	if got != "/v1beta/models/gemini-pro:streamGenerateContent" {
		t.Fatalf("gemini stream default model: %s", got)
	}
	msg := UnsupportedMessage(config.ProtocolOpenAIChat, config.ProtocolGemini)
	if !strings.Contains(msg, string(config.ProtocolOpenAIChat)) || !strings.Contains(msg, string(config.ProtocolGemini)) {
		t.Fatalf("unsupported message: %s", msg)
	}
	if got := ModelFromBody([]byte(`{"model":" gpt-x "}`)); got != "gpt-x" {
		t.Fatalf("model: %q", got)
	}
}

func assertUpstreamRequestShape(t *testing.T, upstream config.Protocol, out []byte) {
	t.Helper()
	s := string(out)
	switch upstream {
	case config.ProtocolOpenAIChat:
		if !strings.Contains(s, `"messages"`) {
			t.Fatalf("want Chat messages:\n%s", s)
		}
		if strings.Contains(s, `"contents"`) || strings.Contains(s, `"candidates"`) {
			t.Fatalf("Gemini leaked into Chat request:\n%s", s)
		}
		if strings.Contains(s, `"object":"response"`) {
			t.Fatalf("Responses leaked into Chat request:\n%s", s)
		}
	case config.ProtocolOpenAIResponses:
		if !strings.Contains(s, `"input"`) {
			t.Fatalf("want Responses input:\n%s", s)
		}
		if strings.Contains(s, `"choices"`) {
			t.Fatalf("Chat leaked into Responses request:\n%s", s)
		}
		if strings.Contains(s, `"contents"`) {
			t.Fatalf("Gemini leaked into Responses request:\n%s", s)
		}
	case config.ProtocolClaudeMessages:
		if !strings.Contains(s, `"messages"`) {
			t.Fatalf("want Claude messages:\n%s", s)
		}
		if strings.Contains(s, `"choices"`) {
			t.Fatalf("Chat leaked into Claude request:\n%s", s)
		}
		if strings.Contains(s, `"contents"`) {
			t.Fatalf("Gemini leaked into Claude request:\n%s", s)
		}
		if strings.Contains(s, `"object":"response"`) {
			t.Fatalf("Responses leaked into Claude request:\n%s", s)
		}
	case config.ProtocolGemini:
		if !strings.Contains(s, `"contents"`) {
			t.Fatalf("want Gemini contents:\n%s", s)
		}
		if strings.Contains(s, `"choices"`) {
			t.Fatalf("Chat leaked into Gemini request:\n%s", s)
		}
		if strings.Contains(s, `"object":"response"`) {
			t.Fatalf("Responses leaked into Gemini request:\n%s", s)
		}
	}
}

func assertClientResponseShape(t *testing.T, client config.Protocol, out []byte) {
	t.Helper()
	s := string(out)
	switch client {
	case config.ProtocolOpenAIChat:
		if !strings.Contains(s, `"choices"`) {
			t.Fatalf("want Chat choices:\n%s", s)
		}
		if strings.Contains(s, `"candidates"`) {
			t.Fatalf("Gemini leaked to Chat client:\n%s", s)
		}
		if strings.Contains(s, `"object":"response"`) {
			t.Fatalf("Responses leaked to Chat client:\n%s", s)
		}
	case config.ProtocolOpenAIResponses:
		if !strings.Contains(s, `"object":"response"`) {
			t.Fatalf("want Responses object:\n%s", s)
		}
		if strings.Contains(s, `"choices"`) {
			t.Fatalf("Chat leaked to Responses client:\n%s", s)
		}
		if strings.Contains(s, `"candidates"`) {
			t.Fatalf("Gemini leaked to Responses client:\n%s", s)
		}
	case config.ProtocolClaudeMessages:
		if !strings.Contains(s, `"type":"message"`) {
			t.Fatalf("want Claude message:\n%s", s)
		}
		if strings.Contains(s, `"choices"`) {
			t.Fatalf("Chat leaked to Claude client:\n%s", s)
		}
		if strings.Contains(s, `"candidates"`) {
			t.Fatalf("Gemini leaked to Claude client:\n%s", s)
		}
		if strings.Contains(s, `"object":"response"`) {
			t.Fatalf("Responses leaked to Claude client:\n%s", s)
		}
	case config.ProtocolGemini:
		if !strings.Contains(s, `"candidates"`) {
			t.Fatalf("want Gemini candidates:\n%s", s)
		}
		if strings.Contains(s, `"choices"`) {
			t.Fatalf("Chat leaked to Gemini client:\n%s", s)
		}
		if strings.Contains(s, `"object":"response"`) {
			t.Fatalf("Responses leaked to Gemini client:\n%s", s)
		}
	}
}
