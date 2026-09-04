package app

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thomas-huang/caosi/internal/config"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestHelpMentionsFlags(t *testing.T) {
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "--help"}, ioDiscard(), &stderr)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	s := stderr.String()
	if !strings.Contains(s, "--port") || !strings.Contains(s, "--config-dir") {
		t.Fatalf("help missing flags:\n%s", s)
	}
	if !strings.Contains(s, "--version") {
		t.Fatalf("help missing --version:\n%s", s)
	}
	if !strings.Contains(s, "github.com/thomas-huang/caosi") {
		t.Fatalf("help missing install path:\n%s", s)
	}
	if !strings.Contains(s, "npm install -g @thomas-huang/caosi") {
		t.Fatalf("help missing npm install:\n%s", s)
	}
	if strings.Contains(s, "guide.html") {
		t.Fatalf("help must not point at guide.html:\n%s", s)
	}
}

func TestVersion_NoConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"caosi", "--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	v := strings.TrimSpace(stdout.String())
	if v == "" {
		t.Fatalf("empty version; stderr=%s", stderr.String())
	}
}

func TestFirstRun_WritesSampleAndExits(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "--config-dir", dir}, ioDiscard(), &stderr)
	if code == 0 {
		t.Fatal("first run must be non-zero")
	}
	path := filepath.Join(dir, config.ProviderFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "//") {
		t.Fatal("sample needs comments")
	}
	out := stderr.String()
	if !strings.Contains(out, "下一步") && !strings.Contains(strings.ToLower(out), "next") {
		if !strings.Contains(out, path) {
			t.Fatalf("message should mention file and next step:\n%s", out)
		}
	}
	if !strings.Contains(out, path) {
		t.Fatalf("message should include wrote path:\n%s", out)
	}
	if strings.Contains(out, "guide.html") {
		t.Fatalf("first-run must not point at guide.html:\n%s", out)
	}
}

func TestRefuseNonLoopback(t *testing.T) {
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "--listen", "0.0.0.0", "--config-dir", t.TempDir()}, ioDiscard(), &stderr)
	if code == 0 {
		t.Fatal("must refuse non-loopback")
	}
	if !strings.Contains(stderr.String(), "127.0.0.1") {
		t.Fatalf("explain loopback:\n%s", stderr.String())
	}
}

func TestMain_ExtraArgs(t *testing.T) {
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "nope"}, ioDiscard(), &stderr)
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "不认识多余参数") {
		t.Fatalf("want extra-arg error:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "--port") {
		t.Fatalf("should print help:\n%s", stderr.String())
	}
}

func TestParseLevel(t *testing.T) {
	if parseLevel("debug") != slog.LevelDebug {
		t.Fatal("debug")
	}
	if parseLevel("WARN") != slog.LevelWarn {
		t.Fatal("warn")
	}
	if parseLevel("error") != slog.LevelError {
		t.Fatal("error")
	}
	if parseLevel("nope") != slog.LevelInfo {
		t.Fatal("default info")
	}
}

func TestMainContext_ReadyTextAndPlaceholder(t *testing.T) {
	dir := t.TempDir()
	raw := `{
  "ds": {
    "base_url": "https://api.example.com",
    "protocol": "openai_chat",
    "api_key": "sk-xxx"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, config.ProviderFileName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stderr := &syncBuf{}
	done := make(chan int, 1)
	go func() {
		done <- MainContext(ctx, []string{"caosi", "--config-dir", dir, "--port", "0", "--log-level", "debug"}, ioDiscard(), stderr)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s := stderr.String()
		if strings.Contains(s, "正在监听") && strings.Contains(s, "占位符") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	s := stderr.String()
	if !strings.Contains(s, "正在监听") {
		cancel()
		t.Fatalf("ready/listen text missing:\n%s", s)
	}
	if !strings.Contains(s, "GET  /health") {
		cancel()
		t.Fatalf("health line missing:\n%s", s)
	}
	if !strings.Contains(s, "ds (openai_chat)") {
		cancel()
		t.Fatalf("provider list missing:\n%s", s)
	}
	if !strings.Contains(s, "占位符") {
		cancel()
		t.Fatalf("placeholder warning missing:\n%s", s)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("MainContext did not return after cancel")
	}
}

func ioDiscard() *bytes.Buffer { return &bytes.Buffer{} }
