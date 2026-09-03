package protocol

import (
	"strings"

	"github.com/thomas-huang/caosi/internal/config"
)

// Detect infers the Client Protocol from the path after the Provider Name.
func Detect(path string) (config.Protocol, bool) {
	p := path
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	switch {
	case strings.HasSuffix(p, "/chat/completions"):
		return config.ProtocolOpenAIChat, true
	case strings.HasSuffix(p, "/responses"):
		return config.ProtocolOpenAIResponses, true
	case strings.HasSuffix(p, "/messages"):
		return config.ProtocolClaudeMessages, true
	case strings.Contains(p, ":generateContent") || strings.Contains(p, ":streamGenerateContent"):
		return config.ProtocolGemini, true
	}
	return "", false
}

func JoinURL(baseURL, remainingPath string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path := remainingPath
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
