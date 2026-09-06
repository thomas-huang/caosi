package header

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/thomas-huang/caosi/internal/config"
)

var hopByHop = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
	"Cookie",
	"Set-Cookie",
}

var clientCreds = []string{
	"Authorization",
	"X-Api-Key",
	"X-Goog-Api-Key",
	"Api-Key",
}

func Apply(dst http.Header, src http.Header, p *config.Provider, upstreamHost string) {
	if src != nil {
		for k, vs := range src {
			if skipIncoming(k, p.Protocol) {
				continue
			}
			for _, v := range vs {
				dst.Add(k, v)
			}
		}
	}
	for _, h := range hopByHop {
		dst.Del(h)
	}
	for _, h := range clientCreds {
		dst.Del(h)
	}
	injectCredential(dst, p)
	for k, v := range p.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if http.CanonicalHeaderKey(k) == "Host" {
			continue
		}
		dst.Set(k, v)
	}
	if dst.Get("Content-Type") == "" {
		dst.Set("Content-Type", "application/json")
	}
	if host := strings.TrimSpace(upstreamHost); host != "" {
		dst.Set("Host", host)
	}
}

// CopyResponse copies upstream response headers for Passthrough, skipping
// hop-by-hop headers and Set-Cookie.
func CopyResponse(dst, src http.Header) {
	if dst == nil || src == nil {
		return
	}
	for k, vs := range src {
		if skipResponse(k) {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func skipResponse(key string) bool {
	ck := http.CanonicalHeaderKey(key)
	for _, h := range hopByHop {
		if ck == http.CanonicalHeaderKey(h) {
			return true
		}
	}
	return false
}

func skipIncoming(key string, upstream config.Protocol) bool {
	ck := http.CanonicalHeaderKey(key)
	lower := strings.ToLower(key)
	switch ck {
	case "Content-Type", "Accept":
		return false
	}
	switch upstream {
	case config.ProtocolClaudeMessages:
		if lower == "anthropic-version" || lower == "anthropic-beta" {
			return false
		}
	case config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses:
		if lower == "openai-organization" || lower == "openai-project" {
			return false
		}
	}
	return true
}

func injectCredential(dst http.Header, p *config.Provider) {
	key := strings.TrimSpace(p.APIKey)
	if key == "" {
		return
	}
	switch p.Protocol {
	case config.ProtocolClaudeMessages:
		dst.Del("Authorization")
		dst.Set("X-Api-Key", key)
		if dst.Get("Anthropic-Version") == "" {
			dst.Set("Anthropic-Version", "2023-06-01")
		}
	case config.ProtocolGemini:
		dst.Del("Authorization")
		dst.Set("X-Goog-Api-Key", key)
	default:
		dst.Del("X-Api-Key")
		dst.Set("Authorization", "Bearer "+key)
	}
}

func UpstreamHost(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Host
}
