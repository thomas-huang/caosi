package app

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
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

func TestMain_NilWritersAndUnknownFlag(t *testing.T) {
	code := Main([]string{"caosi", "--version"}, nil, nil)
	if code != 0 {
		t.Fatalf("nil writers --version exit %d", code)
	}
	var stderr bytes.Buffer
	code = Main([]string{"caosi", "--nope"}, ioDiscard(), &stderr)
	if code != 2 {
		t.Fatalf("unknown flag exit %d", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("unknown flag should write to stderr")
	}
}

func TestMain_EmptyArgsHelpViaListenRefuse(t *testing.T) {
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "--listen", "10.0.0.1"}, ioDiscard(), &stderr)
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "10.0.0.1") {
		t.Fatalf("want refused address:\n%s", stderr.String())
	}
}

func TestMain_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.ProviderFileName), []byte(`{`), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "--config-dir", dir}, ioDiscard(), &stderr)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "配置无效") {
		t.Fatalf("want invalid config:\n%s", stderr.String())
	}
}

func TestMain_ConfigDirIsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "--config-dir", path}, ioDiscard(), &stderr)
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stderr.String(), "无法创建配置") {
		t.Fatalf("want create error:\n%s", stderr.String())
	}
}

func TestMain_ListenInUse(t *testing.T) {
	dir := t.TempDir()
	raw := `{
  "ds": {
    "base_url": "https://api.example.com",
    "protocol": "openai_chat",
    "api_key": "sk-real"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, config.ProviderFileName), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	var stderr bytes.Buffer
	code := Main([]string{"caosi", "--config-dir", dir, "--port", strconv.Itoa(port)}, ioDiscard(), &stderr)
	if code != 1 {
		t.Fatalf("exit %d\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "无法监听") {
		t.Fatalf("want listen error:\n%s", stderr.String())
	}
}

func TestVersionString_Override(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })
	Version = "1.2.3"
	if got := versionString(); got != "1.2.3" {
		t.Fatalf("got %q", got)
	}
	Version = "   "
	if got := versionString(); got == "" {
		t.Fatal("blank version should still yield a non-empty fallback")
	}
}

func TestIsLoopback_LocalhostAndIPv6(t *testing.T) {
	if !isLoopback("localhost") || !isLoopback("::1") || !isLoopback("127.0.0.1") {
		t.Fatal("loopback hosts")
	}
	if isLoopback("8.8.8.8") || isLoopback("not-an-ip") {
		t.Fatal("non-loopback")
	}
}

func TestPrintReady_IPv6Any(t *testing.T) {
	var buf bytes.Buffer
	file := &config.File{Providers: map[string]*config.Provider{
		"ds": {Name: "ds", Protocol: config.ProtocolOpenAIChat, APIKey: "sk-real"},
	}}
	printReady(&buf, "[::]:9999", file)
	s := buf.String()
	if !strings.Contains(s, "http://127.0.0.1:9999") {
		t.Fatalf("want rewritten host:\n%s", s)
	}
	if !strings.Contains(s, "ds (openai_chat)") {
		t.Fatalf("provider:\n%s", s)
	}
}

func TestMainContext_LocalhostListen(t *testing.T) {
	dir := t.TempDir()
	raw := `{
  "ds": {
    "base_url": "https://api.example.com",
    "protocol": "openai_chat",
    "api_key": "sk-live"
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
		done <- MainContext(ctx, []string{"caosi", "--config-dir", dir, "--port", "0", "--listen", "localhost", "--log-level", "warn"}, nil, stderr)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(stderr.String(), "正在监听") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(stderr.String(), "正在监听") {
		cancel()
		t.Fatalf("ready missing:\n%s", stderr.String())
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d\n%s", code, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not return")
	}
}
