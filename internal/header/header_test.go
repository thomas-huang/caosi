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
