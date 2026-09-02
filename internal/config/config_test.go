package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripJSONC_CommentsAndTrailingComma(t *testing.T) {
	in := []byte(`{
  // line comment
  "deepseek": {
    "base_url": "https://api.deepseek.com", /* block */
    "protocol": "openai_chat",
    "api_key": "sk-test",
  },
}`)
	out, err := StripJSONC(in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "line comment") || strings.Contains(string(out), "block") {
		t.Fatalf("comments left: %s", out)
	}
	var f File
	dir := t.TempDir()
	path := filepath.Join(dir, ProviderFileName)
	if err := os.WriteFile(path, in, 0o600); err != nil {
		t.Fatal(err)
	}
	f2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := f2.Providers["deepseek"]
	if p == nil || p.APIKey != "sk-test" || p.Protocol != ProtocolOpenAIChat {
		t.Fatalf("loaded %#v", p)
	}
	_ = f
}

func TestLoad_RejectsHealthName(t *testing.T) {
	dir := t.TempDir()
	raw := `{
  "health": {
    "base_url": "https://example.com",
    "protocol": "openai_chat",
    "api_key": "x"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ProviderFileName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil || !strings.Contains(err.Error(), "保留") {
		t.Fatalf("want reserved-name error, got %v", err)
	}
}

func TestLoad_RejectsUnknownProtocol(t *testing.T) {
	dir := t.TempDir()
	raw := `{"p":{"base_url":"https://x","protocol":"cohere"}}`
	if err := os.WriteFile(filepath.Join(dir, ProviderFileName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(dir)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEnsureProviderFile_WritesCommentedSample(t *testing.T) {
	dir := t.TempDir()
	first, err := EnsureProviderFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil {
		t.Fatal("expected first-run write")
	}
	b, err := os.ReadFile(first.WrotePath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "//") {
		t.Fatal("sample must include comments")
	}
	if !strings.Contains(s, "openai_chat") {
		t.Fatal("sample must mention protocol")
	}
	second, err := EnsureProviderFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if second != nil {
		t.Fatal("second call should not rewrite")
	}
}

func TestSampleParsesAfterStrip(t *testing.T) {
	stripped, err := StripJSONC([]byte(SampleProviderFile))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ProviderFileName)
	if err := os.WriteFile(path, stripped, 0o600); err != nil {
		t.Fatal(err)
	}
	// Sample uses placeholder key — Load should succeed.
	f, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f.Providers["deepseek"] == nil {
		t.Fatal("missing deepseek")
	}
}
