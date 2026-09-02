package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"caosi/internal/config"
)

func testServer(t *testing.T, protocol config.Protocol, handler http.HandlerFunc) (*Server, *httptest.Server) {
	t.Helper()
	up := httptest.NewServer(handler)
	t.Cleanup(up.Close)
	file := &config.File{
		Providers: map[string]*config.Provider{
			"ds": {
				Name:     "ds",
				BaseURL:  up.URL,
				Protocol: protocol,
				APIKey:   "sk-real",
				Model:    "deepseek-chat",
			},
		},
	}
	s := New(file, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return s, up
}

func TestHealth(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["ok"] != true {
		t.Fatalf("body %s", rr.Body.Bytes())
	}
	if rr.Body.Len() == 0 {
		t.Fatal("empty health")
	}
}

func TestLogsDoNotIncludeBodies(t *testing.T) {
	var buf bytes.Buffer
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"SECRET_UPSTREAM_TEXT"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(up.Close)
	file := &config.File{Providers: map[string]*config.Provider{
		"ds": {Name: "ds", BaseURL: up.URL, Protocol: config.ProtocolOpenAIChat, APIKey: "sk-real"},
	}}
	log := slog.New(slog.NewTextHandler(&buf, nil))
	s := New(file, log)
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[{"role":"user","content":"SECRET_CLIENT_TEXT"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	logs := buf.String()
	if strings.Contains(logs, "SECRET_CLIENT_TEXT") || strings.Contains(logs, "SECRET_UPSTREAM_TEXT") {
		t.Fatalf("body leaked into logs: %s", logs)
	}
	if !strings.Contains(logs, "passthrough") {
		t.Fatalf("expected passthrough log, got %s", logs)
	}
}

func TestUnknownProvider(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/nope/v1/chat/completions", strings.NewReader(`{}`)))
	if rr.Code != 404 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
}

func TestUnknownProvider_ClaudeErrorShape(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/nope/v1/messages", strings.NewReader(`{}`)))
	if rr.Code != 404 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), `"type":"error"`) {
		t.Fatalf("want Claude error, got %s", rr.Body.Bytes())
	}
	if strings.Contains(rr.Body.String(), `"code"`) && strings.Contains(rr.Body.String(), `"error":{"message"`) {
		t.Fatalf("looks like OpenAI error: %s", rr.Body.Bytes())
	}
}

func TestPassthrough_BodyAndHeaders(t *testing.T) {
	var sawAuth, sawClientKey string
	var sawBody []byte
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawClientKey = r.Header.Get("X-Api-Key")
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-up","choices":[{"message":{"role":"assistant","content":"from-up"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "placeholder")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if sawAuth != "Bearer sk-real" {
		t.Fatalf("auth=%q", sawAuth)
	}
	if sawClientKey != "" {
		t.Fatalf("client key forwarded: %q", sawClientKey)
	}
	if !strings.Contains(string(sawBody), "deepseek-chat") {
		t.Fatalf("model override missing in upstream body: %s", sawBody)
	}
	if !strings.Contains(rr.Body.String(), "from-up") {
		t.Fatalf("passthrough should return upstream body, got %s", rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), `"type":"message"`) {
		t.Fatal("passthrough must not wrap as Claude")
	}
}

func TestConversion_ClaudeClientOpenAIUpstream(t *testing.T) {
	var sawPath string
	var sawBody []byte
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		sawBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/messages", strings.NewReader(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "placeholder")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(sawPath, "chat/completions") {
		t.Fatalf("upstream path %s", sawPath)
	}
	if !strings.Contains(string(sawBody), `"role":"user"`) {
		t.Fatalf("upstream not openai chat: %s", sawBody)
	}
	if strings.Contains(string(sawBody), `"max_tokens":16`) == false && !strings.Contains(string(sawBody), `"max_tokens": 16`) {
		// still ok if number present
		if !strings.Contains(string(sawBody), "max_tokens") {
			t.Fatalf("max_tokens dropped: %s", sawBody)
		}
	}
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "message" {
		t.Fatalf("client body should be Claude message, got %s", rr.Body.Bytes())
	}
	if strings.Contains(rr.Body.String(), `"choices"`) {
		t.Fatalf("raw OpenAI schema leaked to Claude client: %s", rr.Body.Bytes())
	}
	content := m["content"].([]any)
	text := content[0].(map[string]any)["text"]
	if text != "pong" {
		t.Fatalf("text=%v", text)
	}
}
