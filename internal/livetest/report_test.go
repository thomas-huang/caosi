package livetest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestReportMarkdownAndHTML(t *testing.T) {
	dir := t.TempDir()
	file := &config.File{Providers: map[string]*config.Provider{
		"gate": {Name: "gate", Protocol: config.ProtocolOpenAIResponses, APIKey: "sk-real"},
	}}
	r := newRecorder(dir, file)
	r.add(event{
		Provider: "gate", Upstream: config.ProtocolOpenAIResponses,
		Client: config.ProtocolOpenAIChat, Outcome: outcomePass,
	})
	r.add(event{
		Provider: "gate", Upstream: config.ProtocolOpenAIResponses,
		Client: config.ProtocolOpenAIChat, Stream: true, Outcome: outcomeFail, Err: "status 400: nope",
	})
	md := r.markdown()
	if !strings.Contains(md, "未配置的上游协议") || !strings.Contains(md, "claude_messages") {
		t.Fatalf("missing protocols:\n%s", md)
	}
	if !strings.Contains(md, "PASS") || !strings.Contains(md, "FAIL") || !strings.Contains(md, "status 400") {
		t.Fatalf("matrix:\n%s", md)
	}
	h := r.html()
	if !strings.Contains(h, `class="PASS"`) || !strings.Contains(h, `class="FAIL"`) {
		t.Fatalf("html:\n%s", h)
	}
	mdPath, htmlPath, err := r.write()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(mdPath) != reportMDName || filepath.Base(htmlPath) != reportHTMLName {
		t.Fatalf("%s %s", mdPath, htmlPath)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatal(err)
	}
}
