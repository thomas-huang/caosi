package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"caosi/internal/config"
)

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

func ioDiscard() *bytes.Buffer { return &bytes.Buffer{} }
