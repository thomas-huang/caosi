package convert

import (
	"encoding/json"
	"strings"
)

type irKind string

const (
	irKindText       irKind = "text"
	irKindThinking   irKind = "thinking"
	irKindImage      irKind = "image"
	irKindDocument   irKind = "document"
	irKindAudio      irKind = "audio"
	irKindVideo      irKind = "video"
	irKindToolCall   irKind = "tool_call"
	irKindToolResult irKind = "tool_result"
)

type irPart struct {
	Kind      irKind
	Text      string
	MIME      string
	Filename  string
	AudioFmt  string
	URL       string
	Data      string
	ID        string
	Name      string
	Args      string
	ToolUseID string
	Nested    []irPart
}

type irMessage struct {
	Role       string
	Parts      []irPart
	ToolCallID string
	Name       string
}

type irTool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}

type irRequest struct {
	Model           string
	MaxTokens       int
	Temperature     *float64
	TopP            *float64
	Stream          bool
	Stop            []string
	Tools           []irTool
	ToolChoice      any
	ReasoningEffort string
	IncludeUsage    bool
	Messages        []irMessage
}

type irResponse struct {
	ID               string
	Model            string
	FinishReason     string
	Parts            []irPart
	PromptTokens     int
	CompletionTokens int
	ErrorMessage     string
	ErrorType        string
}

func mediaPart(kind irKind, mime, data, url, filename string) irPart {
	return irPart{Kind: kind, MIME: mime, Data: data, URL: url, Filename: filename}
}

func textPart(s string) irPart {
	return irPart{Kind: irKindText, Text: s}
}

func thinkingPart(s string) irPart {
	return irPart{Kind: irKindThinking, Text: s}
}

func classifyMIME(mime string) irKind {
	m := strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(m, "image/"):
		return irKindImage
	case strings.HasPrefix(m, "audio/"):
		return irKindAudio
	case strings.HasPrefix(m, "video/"):
		return irKindVideo
	default:
		return irKindDocument
	}
}

func parseDataURL(url string) (mime, data string) {
	if !strings.HasPrefix(url, "data:") {
		return "", ""
	}
	rest := strings.TrimPrefix(url, "data:")
	i := strings.Index(rest, ";base64,")
	if i < 0 {
		return "", ""
	}
	return rest[:i], rest[i+len(";base64,"):]
}

func encodeDataURL(mime, data string) string {
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + data
}

func isHTTPURL(url string) bool {
	return strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://")
}

func filenameFromMIME(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "application/pdf":
		return "document.pdf"
	case "text/plain":
		return "document.txt"
	case "text/csv":
		return "document.csv"
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return "document.docx"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return "document.xlsx"
	default:
		return "document.bin"
	}
}

func audioFormatFromMIME(mime string) string {
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "audio/wav", "audio/wave", "audio/x-wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	default:
		return ""
	}
}

func mimeFromAudioFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	default:
		return ""
	}
}

func irText(parts []irPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Kind == irKindText {
			b.WriteString(p.Text)
		}
		for _, n := range p.Nested {
			if n.Kind == irKindText {
				b.WriteString(n.Text)
			}
		}
	}
	return b.String()
}

func irThinking(parts []irPart) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Kind == irKindThinking {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

func splitIRToolCalls(parts []irPart) (rest []irPart, calls []irPart) {
	for _, p := range parts {
		if p.Kind == irKindToolCall {
			calls = append(calls, p)
		} else {
			rest = append(rest, p)
		}
	}
	return rest, calls
}
