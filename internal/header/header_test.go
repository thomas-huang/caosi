package header

import (
	"net/http"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestApply_StripsClientCredsAndInjectsBearer(t *testing.T) {
	p := &config.Provider{
		Name:     "ds",
		BaseURL:  "https://api.deepseek.com",
		Protocol: config.ProtocolOpenAIChat,
		APIKey:   "sk-real",
		Headers:  map[string]string{"HTTP-Referer": "https://local"},
	}
	src := make(http.Header)
	src.Set("X-Api-Key", "placeholder")
	src.Set("Authorization", "Bearer sk-client")
	src.Set("Content-Type", "application/json")
	src.Set("Anthropic-Beta", "should-drop")
	dst := make(http.Header)
	Apply(dst, src, p, "api.deepseek.com")
	if dst.Get("X-Api-Key") != "" {
		t.Fatalf("client x-api-key leaked: %q", dst.Get("X-Api-Key"))
	}
	if dst.Get("Authorization") != "Bearer sk-real" {
		t.Fatalf("auth=%q", dst.Get("Authorization"))
	}
	if dst.Get("HTTP-Referer") != "https://local" {
		t.Fatalf("custom header missing")
	}
	if dst.Get("Anthropic-Beta") != "" {
		t.Fatal("anthropic-beta must not go to OpenAI upstream")
	}
	if dst.Get("Host") != "api.deepseek.com" {
		t.Fatalf("host=%q", dst.Get("Host"))
	}
}

func TestCopyResponse_StripsHopByHopAndSetCookie(t *testing.T) {
	src := make(http.Header)
	src.Set("X-Request-Id", "abc")
	src.Set("Set-Cookie", "sid=1")
	src.Set("Upgrade", "websocket")
	src.Set("Connection", "close")
	src.Add("TE", "trailers")
	dst := make(http.Header)
	CopyResponse(dst, src)
	if dst.Get("X-Request-Id") != "abc" {
		t.Fatalf("x-request-id=%q", dst.Get("X-Request-Id"))
	}
	if dst.Get("Set-Cookie") != "" || dst.Get("Upgrade") != "" || dst.Get("Connection") != "" || dst.Get("TE") != "" {
		t.Fatalf("hop-by-hop leaked: %v", dst)
	}
}

func TestApply_GeminiUsesGoogAPIKey(t *testing.T) {
	p := &config.Provider{Protocol: config.ProtocolGemini, APIKey: "gkey"}
	src := make(http.Header)
	src.Set("Authorization", "Bearer nope")
	src.Set("X-Goog-Api-Key", "client-key")
	dst := make(http.Header)
	Apply(dst, src, p, "generativelanguage.googleapis.com")
	if dst.Get("X-Goog-Api-Key") != "gkey" {
		t.Fatalf("x-goog-api-key=%q", dst.Get("X-Goog-Api-Key"))
	}
	if dst.Get("Authorization") != "" {
		t.Fatalf("authorization should be cleared, got %q", dst.Get("Authorization"))
	}
}

func TestUpstreamHost(t *testing.T) {
	if got := UpstreamHost("https://generativelanguage.googleapis.com/v1beta"); got != "generativelanguage.googleapis.com" {
		t.Fatalf("host=%q", got)
	}
	if got := UpstreamHost("http://127.0.0.1:8080/v1"); got != "127.0.0.1:8080" {
		t.Fatalf("hostport=%q", got)
	}
	if got := UpstreamHost("http://[::1"); got != "" {
		t.Fatalf("invalid URL should yield empty host, got %q", got)
	}
}

func TestApply_ClaudeUsesXApiKey(t *testing.T) {
	p := &config.Provider{Protocol: config.ProtocolClaudeMessages, APIKey: "sk-ant"}
	src := make(http.Header)
	src.Set("Authorization", "Bearer nope")
	src.Set("Anthropic-Beta", "thinking-2024")
	dst := make(http.Header)
	Apply(dst, src, p, "api.anthropic.com")
	if dst.Get("X-Api-Key") != "sk-ant" {
		t.Fatalf("x-api-key=%q", dst.Get("X-Api-Key"))
	}
	if dst.Get("Authorization") != "" {
		t.Fatalf("authorization should be cleared, got %q", dst.Get("Authorization"))
	}
	if dst.Get("Anthropic-Beta") != "thinking-2024" {
		t.Fatalf("beta=%q", dst.Get("Anthropic-Beta"))
	}
}

func TestApply_EmptyKeyNilSrcAndSkippedHeaders(t *testing.T) {
	p := &config.Provider{
		Protocol: config.ProtocolOpenAIChat,
		APIKey:   "",
		Headers:  map[string]string{"": "x", "Host": "evil.example", "X-Custom": "yes"},
	}
	dst := make(http.Header)
	Apply(dst, nil, p, "")
	if dst.Get("Authorization") != "" {
		t.Fatalf("empty key should not inject auth, got %q", dst.Get("Authorization"))
	}
	if dst.Get("Content-Type") != "application/json" {
		t.Fatalf("default content-type=%q", dst.Get("Content-Type"))
	}
	if dst.Get("Host") == "evil.example" {
		t.Fatal("Host override must be skipped")
	}
	if dst.Get("X-Custom") != "yes" {
		t.Fatal("custom header missing")
	}

	src := make(http.Header)
	src.Set("OpenAI-Organization", "org-1")
	src.Set("OpenAI-Project", "proj-1")
	src.Set("Accept", "application/json")
	dst = make(http.Header)
	p.APIKey = "sk"
	Apply(dst, src, p, "api.example.com")
	if dst.Get("OpenAI-Organization") != "org-1" || dst.Get("OpenAI-Project") != "proj-1" {
		t.Fatalf("openai org/project dropped: %v", dst)
	}
	if dst.Get("Host") != "api.example.com" {
		t.Fatalf("host=%q", dst.Get("Host"))
	}
	if dst.Get("Authorization") != "Bearer sk" {
		t.Fatalf("auth=%q", dst.Get("Authorization"))
	}
}

func TestCopyResponse_Nil(t *testing.T) {
	CopyResponse(nil, make(http.Header))
	CopyResponse(make(http.Header), nil)
}

func TestApply_ClaudeKeepsExistingVersion(t *testing.T) {
	p := &config.Provider{Protocol: config.ProtocolClaudeMessages, APIKey: "k"}
	src := make(http.Header)
	src.Set("Anthropic-Version", "2024-01-01")
	dst := make(http.Header)
	Apply(dst, src, p, "")
	if dst.Get("Anthropic-Version") != "2024-01-01" {
		t.Fatalf("version overwritten: %q", dst.Get("Anthropic-Version"))
	}
}
