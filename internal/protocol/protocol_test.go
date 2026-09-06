package protocol

import (
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		path string
		want config.Protocol
		ok   bool
	}{
		{"/v1/chat/completions", config.ProtocolOpenAIChat, true},
		{"/v1/chat/completions?stream=true", config.ProtocolOpenAIChat, true},
		{"/v1/responses", config.ProtocolOpenAIResponses, true},
		{"/v1/messages", config.ProtocolClaudeMessages, true},
		{"/v1/messages?beta=1", config.ProtocolClaudeMessages, true},
		{"/v1beta/models/gemini-pro:generateContent", config.ProtocolGemini, true},
		{"/v1beta/models/gemini-pro:generateContent?alt=sse", config.ProtocolGemini, true},
		{"/v1beta/models/gemini-pro:streamGenerateContent", config.ProtocolGemini, true},
		{"/foo", "", false},
		{"/v1/messages/count_tokens", "", false},
		{"v1/chat/completions", config.ProtocolOpenAIChat, true},
		{"foo", "", false},
	}
	for _, tc := range cases {
		got, ok := Detect(tc.path)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("%s: got %s %v want %s %v", tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestJoinURL_NoMagicV1(t *testing.T) {
	got := JoinURL("https://api.deepseek.com", "/v1/chat/completions")
	if got != "https://api.deepseek.com/v1/chat/completions" {
		t.Fatal(got)
	}
	got = JoinURL("https://api.openai.com/v1", "/v1/chat/completions")
	if got != "https://api.openai.com/v1/v1/chat/completions" {
		t.Fatalf("must not strip /v1, got %s", got)
	}
	got = JoinURL("https://api.deepseek.com/", "/v1/chat/completions?foo=1")
	if got != "https://api.deepseek.com/v1/chat/completions" {
		t.Fatalf("trim slash and drop query, got %s", got)
	}
	got = JoinURL("https://api.deepseek.com", "v1/messages")
	if got != "https://api.deepseek.com/v1/messages" {
		t.Fatalf("missing leading slash, got %s", got)
	}
	got = JoinURL("https://api.deepseek.com", "")
	if got != "https://api.deepseek.com/" {
		t.Fatalf("empty path, got %s", got)
	}
}
