package livetest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestDuplicateTopLevelKeys(t *testing.T) {
	dups, err := duplicateTopLevelKeys([]byte(`{"a":{"x":1},"b":{"x":2},"a":{"x":3}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(dups) != 1 || dups[0] != "a" {
		t.Fatalf("dups=%v", dups)
	}
	dups, err = duplicateTopLevelKeys([]byte(`{"a":{"x":1},"b":{"x":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(dups) != 0 {
		t.Fatalf("dups=%v", dups)
	}
}

func TestIsPlaceholderKey(t *testing.T) {
	if !isPlaceholderKey("") || !isPlaceholderKey("sk-xxx") || !isPlaceholderKey("sk-your-key-here") {
		t.Fatal("want placeholders")
	}
	if isPlaceholderKey("sk-real-token") {
		t.Fatal("real key")
	}
}

func TestCheckSnapshot_AllowsMissingProtocol(t *testing.T) {
	file := &config.File{Providers: map[string]*config.Provider{
		"a": {Name: "a", Protocol: config.ProtocolOpenAIChat, APIKey: "sk-real"},
	}}
	if err := checkSnapshot(file); err != nil {
		t.Fatal(err)
	}
	miss := missingUpstream(file)
	if len(miss) != 3 {
		t.Fatalf("missing=%v", miss)
	}
}

func TestCheckSnapshot_OnlyPlaceholders(t *testing.T) {
	file := &config.File{Providers: map[string]*config.Provider{
		"a": {Name: "a", Protocol: config.ProtocolOpenAIChat, APIKey: "sk-xxx"},
	}}
	err := checkSnapshot(file)
	if err == nil || !strings.Contains(err.Error(), "没有可用") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadLiveFile_DuplicateAndPlaceholder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.ProviderFileName)
	raw := `{
  "p": {"base_url": "https://example.com", "protocol": "openai_chat", "api_key": "sk-real"},
  "p": {"base_url": "https://example.com", "protocol": "gemini", "api_key": "sk-real"}
}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadLiveFile(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("got %v", err)
	}

	ok := `{
  "chat": {"base_url": "https://example.com", "protocol": "openai_chat", "api_key": "sk-xxx"},
  "resp": {"base_url": "https://example.com", "protocol": "openai_responses", "api_key": "sk-real"},
  "claude": {"base_url": "https://example.com", "protocol": "claude_messages", "api_key": "sk-real"},
  "gem": {"base_url": "https://example.com", "protocol": "gemini", "api_key": "sk-real"}
}`
	if err := os.WriteFile(path, []byte(ok), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := loadLiveFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := placeholderNames(file); len(got) != 1 || got[0] != "chat" {
		t.Fatalf("placeholders=%v", got)
	}
	if got := usableNames(file); len(got) != 3 {
		t.Fatalf("usable=%v", got)
	}
}

func TestLoadLiveFile_Missing(t *testing.T) {
	_, err := loadLiveFile(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("got %v", err)
	}
}

func TestApplies_Media(t *testing.T) {
	if applies(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, cellAudio) {
		t.Fatal("claude cannot send audio")
	}
	if applies(config.ProtocolGemini, config.ProtocolClaudeMessages, cellAudio) {
		t.Fatal("claude upstream has no audio analogue")
	}
	if !applies(config.ProtocolGemini, config.ProtocolGemini, cellVideo) {
		t.Fatal("gemini video")
	}
	if applies(config.ProtocolOpenAIChat, config.ProtocolGemini, cellVideo) {
		t.Fatal("chat does not send video_url")
	}
	if !applies(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, cellImage) {
		t.Fatal("image always")
	}
}
