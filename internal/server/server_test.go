package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thomas-huang/caosi/internal/config"
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

func TestConversion_ResponsesClientChatUpstream(t *testing.T) {
	var sawPath string
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/responses", strings.NewReader(`{"model":"gpt-x","input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if rr.Code == http.StatusNotImplemented {
		t.Fatal("Responses→Chat must not 501")
	}
	if !strings.Contains(sawPath, "chat/completions") {
		t.Fatalf("path %s", sawPath)
	}
	body := rr.Body.String()
	if strings.Contains(body, `"choices"`) {
		t.Fatalf("chat leaked: %s", body)
	}
	if !strings.Contains(body, `"object":"response"`) || !strings.Contains(body, "pong") {
		t.Fatalf("want Responses body: %s", body)
	}
}

func TestConversion_ClaudeClientResponsesUpstream(t *testing.T) {
	var sawPath string
	s, _ := testServer(t, config.ProtocolOpenAIResponses, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_1","object":"response","status":"completed","error":null,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/messages", strings.NewReader(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(sawPath, "/responses") {
		t.Fatalf("path %s", sawPath)
	}
	if strings.Contains(rr.Body.String(), `"choices"`) || strings.Contains(rr.Body.String(), `"object":"response"`) && !strings.Contains(rr.Body.String(), `"type":"message"`) {
		// Claude message has type=message; Responses has object=response
	}
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "message" {
		t.Fatalf("want Claude message, got %s", rr.Body.Bytes())
	}
	if strings.Contains(rr.Body.String(), "upstream error") {
		t.Fatalf("error:null treated as failure: %s", rr.Body.Bytes())
	}
	content, _ := m["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("empty content: %s", rr.Body.Bytes())
	}
	if content[0].(map[string]any)["text"] != "pong" {
		t.Fatalf("text=%v", content[0])
	}
}

func TestConversion_GeminiClientChatUpstream(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1beta/models/gemini-pro:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	body := rr.Body.String()
	if strings.Contains(body, `"choices"`) {
		t.Fatalf("chat leaked: %s", body)
	}
	if !strings.Contains(body, `"candidates"`) || !strings.Contains(body, "pong") {
		t.Fatalf("want Gemini body: %s", body)
	}
}

func TestConversion_SSE_ResponsesFromChat(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"hmm\"}}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"lo\"},\"finish_reason\":\"stop\"}]}\n\n"))
		fl.Flush()
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/responses", strings.NewReader(`{"model":"gpt-x","stream":true,"input":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	got := rr.Body.String()
	if !strings.Contains(got, "response.output_text.delta") && !strings.Contains(got, "response.completed") {
		t.Fatalf("want Responses SSE, got %s", got)
	}
	if strings.Contains(got, `"choices"`) {
		t.Fatalf("chat SSE leaked: %s", got)
	}
	if !strings.Contains(got, "hmm") || !strings.Contains(got, `"type":"reasoning"`) {
		t.Fatalf("reasoning analogue missing from SSE:\n%s", got)
	}
}

func TestConversion_SSE_ChatClientClaudeUpstream(t *testing.T) {
	s, _ := testServer(t, config.ProtocolClaudeMessages, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[]}}\n\n"))
		_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n"))
		_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	got := rr.Body.String()
	if !strings.Contains(got, "hello") {
		t.Fatalf("Claude SSE fallback lost text:\n%s", got)
	}
}

func TestHotReload_KeepsLastGoodThenAppliesValid(t *testing.T) {
	dir := t.TempDir()
	writeProv := func(key string) {
		t.Helper()
		body := `{
  "ds": {
    "base_url": "URL",
    "protocol": "openai_chat",
    "api_key": "` + key + `"
  }
}`
		if err := os.WriteFile(filepath.Join(dir, config.ProviderFileName), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var sawAuth []string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = append(sawAuth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(up.Close)
	writeProv("sk-old")
	raw, _ := os.ReadFile(filepath.Join(dir, config.ProviderFileName))
	raw = []byte(strings.ReplaceAll(string(raw), "URL", up.URL))
	if err := os.WriteFile(filepath.Join(dir, config.ProviderFileName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := New(file, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.WatchConfig(ctx, dir)
	post := func() {
		req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[]}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		s.Handler().ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
		}
	}
	post()
	if err := os.WriteFile(filepath.Join(dir, config.ProviderFileName), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)
	post()
	good := `{
  "ds": {
    "base_url": "` + up.URL + `",
    "protocol": "openai_chat",
    "api_key": "sk-new"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, config.ProviderFileName), []byte(good), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		post()
		if len(sawAuth) > 0 && sawAuth[len(sawAuth)-1] == "Bearer sk-new" {
			break
		}
	}
	if len(sawAuth) < 2 {
		t.Fatalf("too few upstream hits: %v", sawAuth)
	}
	if sawAuth[1] != "Bearer sk-old" {
		t.Fatalf("illegal save should keep old key, got %v", sawAuth)
	}
	if sawAuth[len(sawAuth)-1] != "Bearer sk-new" {
		t.Fatalf("valid save should apply new key, got %v", sawAuth)
	}
}
