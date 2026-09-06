package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/thomas-huang/caosi/internal/config"
	"github.com/thomas-huang/caosi/internal/convert"
	"github.com/thomas-huang/caosi/internal/header"
	"github.com/thomas-huang/caosi/internal/protocol"
)

var maxBody int64 = 32 << 20

type Server struct {
	mu     sync.RWMutex
	file   *config.File
	log    *slog.Logger
	client *http.Client
}

func New(file *config.File, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{
		file: file,
		log:  log,
		client: &http.Client{
			Timeout: 0,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DisableCompression:    true,
				MaxIdleConns:          32,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   15 * time.Second,
				ResponseHeaderTimeout: 10 * time.Minute,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleProxy)
	return mux
}

func (s *Server) File() *config.File {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.file
}

func (s *Server) ReplaceFile(f *config.File) {
	s.mu.Lock()
	s.file = f
	s.mu.Unlock()
}

// WatchConfig polls the Provider File and swaps in a newly loaded map.
// Illegal saves are logged and the last good File is kept.
func (s *Server) WatchConfig(ctx context.Context, configDir string) {
	path := config.ProviderFilePath(configDir)
	var lastMod time.Time
	if st, err := os.Stat(path); err == nil {
		lastMod = st.ModTime()
	}
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			st, err := os.Stat(path)
			if err != nil || !st.ModTime().After(lastMod) {
				continue
			}
			lastMod = st.ModTime()
			f, err := config.Load(configDir)
			if err != nil {
				s.log.Error("reload failed; keeping last good config", "err", err)
				continue
			}
			s.ReplaceFile(f)
			s.log.Info("reloaded providers")
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	names := config.Names(s.File())
	body, _ := json.Marshal(map[string]any{
		"ok":        true,
		"providers": names,
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	path := r.URL.Path
	if path == "/" {
		s.writeClientError(w, "", http.StatusNotFound,
			"缺少 Provider Name。用法: http://127.0.0.1:9999/{provider_name}/v1/...")
		return
	}
	name, rest := splitProvider(path)
	clientProto, detected := protocol.Detect(rest)
	file := s.File()
	p := file.Providers[name]
	if p == nil {
		s.writeClientError(w, clientProto, http.StatusNotFound,
			"没有叫 "+name+" 的 Provider。检查 providers.jsonc 里的 key，或打开 /health 看当前列表。")
		return
	}
	if !detected {
		s.writeClientError(w, clientProto, http.StatusNotFound,
			"认不出客户端协议。这一版支持 /v1/chat/completions、/v1/responses、/v1/messages、Gemini :generateContent。")
		return
	}
	if !convert.Supported(clientProto, p.Protocol) {
		status, body, ct := convert.ClientError(clientProto, http.StatusNotImplemented, convert.UnsupportedMessage(clientProto, p.Protocol))
		w.Header().Set("Content-Type", ct)
		w.WriteHeader(status)
		_, _ = w.Write(body)
		s.logReq(r.Context(), p.Name, clientProto, p.Protocol, status, time.Since(start), convert.NeedsConvert(clientProto, p.Protocol))
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil {
		s.writeClientError(w, clientProto, http.StatusBadRequest, "读请求失败: "+err.Error())
		return
	}
	if int64(len(body)) > maxBody {
		s.writeClientError(w, clientProto, http.StatusRequestEntityTooLarge, "请求体超过 32MiB")
		return
	}

	stream := wantsStream(body, rest)
	upBody := body
	if convert.NeedsConvert(clientProto, p.Protocol) || p.Model != "" {
		upBody, err = convert.Request(clientProto, p.Protocol, body, p.Model, stream)
		if err != nil {
			s.writeClientError(w, clientProto, http.StatusBadRequest, err.Error())
			return
		}
	}

	model := p.Model
	if model == "" {
		model = convert.ModelFromBody(body)
	}
	upPath := convert.UpstreamPath(clientProto, p.Protocol, rest, model, stream)
	upURL := protocol.JoinURL(p.BaseURL, upPath)
	if r.URL.RawQuery != "" && !convert.NeedsConvert(clientProto, p.Protocol) {
		upURL = upURL + "?" + r.URL.RawQuery
	} else if p.Protocol == config.ProtocolGemini && stream && convert.NeedsConvert(clientProto, p.Protocol) {
		upURL += "?alt=sse"
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, upURL, bytes.NewReader(upBody))
	if err != nil {
		s.writeClientError(w, clientProto, http.StatusBadGateway, "无法构造上游请求: "+err.Error())
		return
	}
	host := header.UpstreamHost(p.BaseURL)
	req.Host = host
	header.Apply(req.Header, r.Header, p, host)
	req.ContentLength = int64(len(upBody))

	resp, err := s.client.Do(req)
	if err != nil {
		s.writeClientError(w, clientProto, http.StatusBadGateway, "上游连接失败: "+err.Error())
		s.logReq(r.Context(), p.Name, clientProto, p.Protocol, http.StatusBadGateway, time.Since(start), convert.NeedsConvert(clientProto, p.Protocol))
		return
	}
	defer resp.Body.Close()

	converting := convert.NeedsConvert(clientProto, p.Protocol)
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBody+1))
		s.writeClientError(w, clientProto, http.StatusBadGateway, "上游返回重定向")
		s.logReq(r.Context(), p.Name, clientProto, p.Protocol, http.StatusBadGateway, time.Since(start), converting)
		return
	}

	ct := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(ct, "text/event-stream") || stream

	if !converting {
		if isSSE {
			header.CopyResponse(w.Header(), resp.Header)
			w.WriteHeader(resp.StatusCode)
			_, _ = io.Copy(&flushWriter{w: w}, resp.Body)
			s.logReq(r.Context(), p.Name, clientProto, p.Protocol, resp.StatusCode, time.Since(start), false)
			return
		}
		upResp, err := readJSONBody(resp.Body)
		if err != nil {
			s.writeClientError(w, clientProto, http.StatusBadGateway, err.Error())
			s.logReq(r.Context(), p.Name, clientProto, p.Protocol, http.StatusBadGateway, time.Since(start), false)
			return
		}
		header.CopyResponse(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(upResp)
		s.logReq(r.Context(), p.Name, clientProto, p.Protocol, resp.StatusCode, time.Since(start), false)
		return
	}

	if isSSE {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(resp.StatusCode)
		err := convert.Stream(clientProto, p.Protocol, resp.Body, &flushWriter{w: w})
		if err != nil && r.Context().Err() == nil {
			s.log.Warn("stream convert", "err", err)
		}
		s.logReq(r.Context(), p.Name, clientProto, p.Protocol, resp.StatusCode, time.Since(start), true)
		return
	}

	upResp, err := readJSONBody(resp.Body)
	if err != nil {
		s.writeClientError(w, clientProto, http.StatusBadGateway, err.Error())
		s.logReq(r.Context(), p.Name, clientProto, p.Protocol, http.StatusBadGateway, time.Since(start), true)
		return
	}
	out, err := convert.Response(clientProto, p.Protocol, upResp)
	if err != nil {
		s.writeClientError(w, clientProto, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(out)
	s.logReq(r.Context(), p.Name, clientProto, p.Protocol, resp.StatusCode, time.Since(start), true)
}

func readJSONBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("读上游响应失败: %w", err)
	}
	if int64(len(body)) > maxBody {
		return nil, fmt.Errorf("响应体超过 32MiB")
	}
	return body, nil
}

func (s *Server) writeClientError(w http.ResponseWriter, client config.Protocol, status int, msg string) {
	if client == "" {
		client = config.ProtocolOpenAIChat
	}
	st, body, ct := convert.ClientError(client, status, msg)
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(st)
	_, _ = w.Write(body)
}

func (s *Server) logReq(_ context.Context, provider string, client, upstream config.Protocol, status int, d time.Duration, conv bool) {
	kind := "passthrough"
	if conv {
		kind = "conversion"
	}
	s.log.Info(fmt.Sprintf("%s  %s→%s  %s  %d  %s",
		provider, client, upstream, kind, status, d.Round(time.Millisecond)))
}

func splitProvider(path string) (name, rest string) {
	p := strings.TrimPrefix(path, "/")
	i := strings.IndexByte(p, '/')
	if i < 0 {
		return p, "/"
	}
	return p[:i], p[i:]
}

func wantsStream(body []byte, path string) bool {
	if strings.Contains(path, "streamGenerateContent") {
		return true
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	v, _ := m["stream"].(bool)
	return v
}

type flushWriter struct {
	w http.ResponseWriter
}

func (f *flushWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
	return n, err
}

func (f *flushWriter) Flush() {
	if fl, ok := f.w.(http.Flusher); ok {
		fl.Flush()
	}
}
