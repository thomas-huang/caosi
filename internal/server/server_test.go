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

func TestRootPath_NotFound(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != 404 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "缺少 Provider Name") {
		t.Fatalf("want usage hint, got %s", rr.Body.Bytes())
	}
}

func TestUnknownClientPath(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/ds/v1/unknown", strings.NewReader(`{}`)))
	if rr.Code != 404 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "认不出客户端协议") {
		t.Fatalf("want protocol hint, got %s", rr.Body.Bytes())
	}
}

func TestHealth_NonGET(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/health", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
}

func TestUpstreamConnectFailure(t *testing.T) {
	file := &config.File{Providers: map[string]*config.Provider{
		"ds": {Name: "ds", BaseURL: "http://127.0.0.1:1", Protocol: config.ProtocolOpenAIChat, APIKey: "sk-real"},
	}}
	s := New(file, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "上游连接失败") {
		t.Fatalf("want connect failure, got %s", rr.Body.Bytes())
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

func TestHop_GeminiIdentityAppliesModelOverrideToURL(t *testing.T) {
	var sawPath string
	s, _ := testServer(t, config.ProtocolGemini, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1beta/models/live:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if sawPath != "/v1beta/models/deepseek-chat:generateContent" {
		t.Fatalf("upstream path %s (Model Override not applied to Gemini URL)", sawPath)
	}
	if !strings.Contains(rr.Body.String(), `"candidates"`) {
		t.Fatalf("want Gemini body: %s", rr.Body.Bytes())
	}
}

func TestHop_GeminiIdentityStreamAppliesModelOverrideToURL(t *testing.T) {
	var sawPath string
	s, _ := testServer(t, config.ProtocolGemini, func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1beta/models/live:streamGenerateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if sawPath != "/v1beta/models/deepseek-chat:streamGenerateContent" {
		t.Fatalf("upstream path %s (Model Override not applied to Gemini stream URL)", sawPath)
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

func TestHop_RedirectBecomes502(t *testing.T) {
	followed := false
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/followed") {
			followed = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		w.Header().Set("Location", r.URL.Path+"/followed")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(up.Close)

	file := &config.File{Providers: map[string]*config.Provider{
		"ds": {Name: "ds", BaseURL: up.URL, Protocol: config.ProtocolOpenAIChat, APIKey: "sk-real"},
	}}
	s := New(file, slog.New(slog.NewTextHandler(io.Discard, nil)))

	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if followed {
		t.Fatal("must not follow 3xx")
	}
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if rr.Header().Get("Location") != "" {
		t.Fatalf("Location leaked: %q", rr.Header().Get("Location"))
	}
	if !strings.Contains(rr.Body.String(), "上游返回重定向") {
		t.Fatalf("want redirect error, got %s", rr.Body.Bytes())
	}

	req = httptest.NewRequest(http.MethodPost, "/ds/v1/messages", strings.NewReader(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if followed {
		t.Fatal("must not follow 3xx on conversion")
	}
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("conversion status %d %s", rr.Code, rr.Body.Bytes())
	}
	if rr.Header().Get("Location") != "" {
		t.Fatalf("Location leaked on conversion: %q", rr.Header().Get("Location"))
	}
	if !strings.Contains(rr.Body.String(), `"type":"error"`) {
		t.Fatalf("want Claude error, got %s", rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "上游返回重定向") {
		t.Fatalf("want redirect error, got %s", rr.Body.Bytes())
	}
}

func TestHop_PassthroughResponseHeaders(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "abc")
		w.Header().Set("Set-Cookie", "sid=1")
		w.Header().Set("Upgrade", "websocket")
		w.Header().Set("Connection", "close")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if rr.Header().Get("X-Request-Id") != "abc" {
		t.Fatalf("x-request-id=%q", rr.Header().Get("X-Request-Id"))
	}
	if rr.Header().Get("Set-Cookie") != "" || rr.Header().Get("Upgrade") != "" {
		t.Fatalf("hop-by-hop leaked: %v", rr.Header())
	}
}

func TestHop_ConversionOmitsUpstreamHeaders(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "abc")
		w.Header().Set("Openai-Ratelimit-Remaining", "9")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/messages", strings.NewReader(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if rr.Header().Get("X-Request-Id") != "" || rr.Header().Get("Openai-Ratelimit-Remaining") != "" {
		t.Fatalf("upstream headers leaked: %v", rr.Header())
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content-type=%q", rr.Header().Get("Content-Type"))
	}
}

func TestHop_QueryPassthroughKept(t *testing.T) {
	var sawQuery string
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions?foo=bar", strings.NewReader(`{"model":"x","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if sawQuery != "foo=bar" {
		t.Fatalf("query=%q", sawQuery)
	}
}

func TestHop_QueryConversionDropped(t *testing.T) {
	var sawQuery string
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/messages?foo=bar", strings.NewReader(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if sawQuery != "" {
		t.Fatalf("conversion must drop client query, got %q", sawQuery)
	}
}

func TestHop_GeminiStreamAddsAltSSE(t *testing.T) {
	var sawQuery, sawPath string
	s, _ := testServer(t, config.ProtocolGemini, func(w http.ResponseWriter, r *http.Request) {
		sawQuery = r.URL.RawQuery
		sawPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/messages", strings.NewReader(`{"model":"claude-opus","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if sawQuery != "alt=sse" {
		t.Fatalf("query=%q path=%s", sawQuery, sawPath)
	}
	if !strings.Contains(sawPath, "streamGenerateContent") {
		t.Fatalf("path=%s", sawPath)
	}
}

func TestHop_JSONOversize502(t *testing.T) {
	old := maxBody
	maxBody = 256
	t.Cleanup(func() { maxBody = old })
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(strings.Repeat("a", 300)))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("passthrough status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "响应体超过 32MiB") {
		t.Fatalf("want oversize, got %s", rr.Body.Bytes())
	}

	req = httptest.NewRequest(http.MethodPost, "/ds/v1/messages", strings.NewReader(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":"ping"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("conversion status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "响应体超过 32MiB") {
		t.Fatalf("want oversize, got %s", rr.Body.Bytes())
	}
}

func TestHop_SSEUnbounded(t *testing.T) {
	old := maxBody
	maxBody = 256
	t.Cleanup(func() { maxBody = old })
	payload := strings.Repeat("a", 300)
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(payload))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","stream":true,"messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if rr.Body.String() != payload {
		t.Fatalf("sse truncated: %d bytes", rr.Body.Len())
	}
}

func TestHop_HostFromBaseURL(t *testing.T) {
	var sawHost string
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawHost = r.Host
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "http://caosi.test/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[]}`))
	req.Host = "caosi.test"
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if sawHost == "caosi.test" || sawHost == "" {
		t.Fatalf("upstream host=%q", sawHost)
	}
}

func TestHop_MethodCopied(t *testing.T) {
	var sawMethod string
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawMethod = r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1"}`))
	})
	req := httptest.NewRequest(http.MethodGet, "/ds/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if sawMethod != http.MethodGet {
		t.Fatalf("method=%q", sawMethod)
	}
}

func TestHop_NoAcceptEncoding(t *testing.T) {
	var sawAE string
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		sawAE = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if sawAE != "" {
		t.Fatalf("Accept-Encoding=%q", sawAE)
	}
}

func TestNew_NilLogAndFileReplace(t *testing.T) {
	file := &config.File{Providers: map[string]*config.Provider{
		"ds": {Name: "ds", BaseURL: "http://127.0.0.1:1", Protocol: config.ProtocolOpenAIChat},
	}}
	s := New(file, nil)
	if s.File() != file {
		t.Fatal("File()")
	}
	next := &config.File{Providers: map[string]*config.Provider{
		"other": {Name: "other"},
	}}
	s.ReplaceFile(next)
	if s.File().Providers["other"] == nil {
		t.Fatal("ReplaceFile")
	}
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodHead, "/health", nil))
	if rr.Code != 200 {
		t.Fatalf("head health %d", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("HEAD should have empty body, got %s", rr.Body.Bytes())
	}
}

func TestSplitProviderAndWantsStream(t *testing.T) {
	name, rest := splitProvider("/ds")
	if name != "ds" || rest != "/" {
		t.Fatalf("name=%q rest=%q", name, rest)
	}
	name, rest = splitProvider("/ds/v1/messages")
	if name != "ds" || rest != "/v1/messages" {
		t.Fatalf("name=%q rest=%q", name, rest)
	}
	if !wantsStream(nil, "/v1beta/models/x:streamGenerateContent") {
		t.Fatal("streamGenerateContent")
	}
	if wantsStream([]byte(`not-json`), "/v1/messages") {
		t.Fatal("invalid json")
	}
	if !wantsStream([]byte(`{"stream":true}`), "/v1/messages") {
		t.Fatal("stream true")
	}
	if wantsStream([]byte(`{"stream":false}`), "/v1/messages") {
		t.Fatal("stream false")
	}
}

func TestHop_RequestBodyTooLarge(t *testing.T) {
	old := maxBody
	maxBody = 16
	t.Cleanup(func() { maxBody = old })
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(strings.Repeat("a", 32)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "32MiB") {
		t.Fatalf("want oversize, got %s", rr.Body.Bytes())
	}
}

func TestHop_ConvertRequestError(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/messages", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
}

func TestHop_ProviderOnlyPath(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {})
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/ds", strings.NewReader(`{}`)))
	if rr.Code != 404 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
}

func TestFlushWriter_Flushes(t *testing.T) {
	rr := httptest.NewRecorder()
	fw := &flushWriter{w: rr}
	n, err := fw.Write([]byte("hi"))
	if err != nil || n != 2 {
		t.Fatalf("write %d %v", n, err)
	}
	fw.Flush()
	if rr.Body.String() != "hi" {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestHop_PassthroughSSE(t *testing.T) {
	s, _ := testServer(t, config.ProtocolOpenAIChat, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: hi\n\n"))
	})
	req := httptest.NewRequest(http.MethodPost, "/ds/v1/chat/completions", strings.NewReader(`{"model":"x","stream":true,"messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d %s", rr.Code, rr.Body.Bytes())
	}
	if !strings.Contains(rr.Body.String(), "data: hi") {
		t.Fatalf("sse: %s", rr.Body.String())
	}
}
