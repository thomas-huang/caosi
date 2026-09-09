package livetest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/thomas-huang/caosi/internal/config"
)

const (
	userText   = "Reply with the single word ok."
	systemText = "You are a terse assistant."
	dummyModel = "live-test"
	maxTokens  = 16
)

func clientPath(p config.Protocol, stream bool) string {
	switch p {
	case config.ProtocolOpenAIChat:
		return "/v1/chat/completions"
	case config.ProtocolOpenAIResponses:
		return "/v1/responses"
	case config.ProtocolClaudeMessages:
		return "/v1/messages"
	case config.ProtocolGemini:
		if stream {
			return "/v1beta/models/live:streamGenerateContent"
		}
		return "/v1beta/models/live:generateContent"
	}
	return ""
}

func bundleCells(client, upstream config.Protocol) []cell {
	var out []cell
	for _, c := range cellOrder {
		if applies(client, upstream, c) {
			out = append(out, c)
		}
	}
	return out
}

func hasCell(cells []cell, want cell) bool {
	for _, c := range cells {
		if c == want {
			return true
		}
	}
	return false
}

func requestBody(client config.Protocol, stream bool, cells []cell) ([]byte, error) {
	if len(cells) == 0 {
		return nil, fmt.Errorf("no cells")
	}
	switch client {
	case config.ProtocolOpenAIChat:
		return chatBundle(cells, stream)
	case config.ProtocolOpenAIResponses:
		return responsesBundle(cells, stream)
	case config.ProtocolClaudeMessages:
		return claudeBundle(cells, stream)
	case config.ProtocolGemini:
		return geminiBundle(cells)
	}
	return nil, fmt.Errorf("unknown Client Protocol %s", client)
}

func chatBundle(cells []cell, stream bool) ([]byte, error) {
	parts := mediaPartsChat(cells)
	var userContent any = userText
	if len(parts) > 0 {
		parts = append(parts, map[string]any{"type": "text", "text": userText})
		userContent = parts
	}
	msgs := make([]any, 0, 2)
	if hasCell(cells, cellSystem) {
		msgs = append(msgs, map[string]any{"role": "system", "content": systemText})
	}
	msgs = append(msgs, map[string]any{"role": "user", "content": userContent})
	m := map[string]any{
		"model":      dummyModel,
		"max_tokens": maxTokens,
		"stream":     stream,
		"messages":   msgs,
	}
	if hasCell(cells, cellTools) {
		m["tools"] = []any{chatTool()}
	}
	return json.Marshal(m)
}

func mediaPartsChat(cells []cell) []any {
	var parts []any
	if hasCell(cells, cellImage) {
		parts = append(parts, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": dataURL("image/png", png16)},
		})
	}
	if hasCell(cells, cellDocument) {
		parts = append(parts, map[string]any{
			"type": "file",
			"file": map[string]any{
				"filename":  "document.pdf",
				"file_data": dataURL("application/pdf", miniPDF()),
			},
		})
	}
	if hasCell(cells, cellAudio) {
		parts = append(parts, map[string]any{
			"type": "input_audio",
			"input_audio": map[string]any{
				"data":   b64(wavSilence()),
				"format": "wav",
			},
		})
	}
	return parts
}

func responsesBundle(cells []cell, stream bool) ([]byte, error) {
	parts := make([]any, 0, 5)
	if hasCell(cells, cellImage) {
		parts = append(parts, map[string]any{
			"type":      "input_image",
			"image_url": dataURL("image/png", png16),
		})
	}
	if hasCell(cells, cellDocument) {
		parts = append(parts, map[string]any{
			"type":      "input_file",
			"filename":  "document.pdf",
			"file_data": dataURL("application/pdf", miniPDF()),
		})
	}
	parts = append(parts, map[string]any{"type": "input_text", "text": userText})
	m := map[string]any{
		"model":             dummyModel,
		"max_output_tokens": maxTokens,
		"stream":            stream,
		"input": []any{
			map[string]any{"role": "user", "content": parts},
		},
	}
	if hasCell(cells, cellSystem) {
		m["instructions"] = systemText
	}
	if hasCell(cells, cellTools) {
		m["tools"] = []any{map[string]any{
			"type":        "function",
			"name":        "get_time",
			"description": "Current time",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		}}
	}
	return json.Marshal(m)
}

func claudeBundle(cells []cell, stream bool) ([]byte, error) {
	parts := make([]any, 0, 5)
	if hasCell(cells, cellImage) {
		parts = append(parts, map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": "image/png",
				"data":       b64(png16),
			},
		})
	}
	if hasCell(cells, cellDocument) {
		parts = append(parts, map[string]any{
			"type": "document",
			"source": map[string]any{
				"type":       "base64",
				"media_type": "application/pdf",
				"data":       b64(miniPDF()),
			},
		})
	}
	var userContent any = userText
	if len(parts) > 0 {
		parts = append(parts, map[string]any{"type": "text", "text": userText})
		userContent = parts
	}
	m := map[string]any{
		"model":      dummyModel,
		"max_tokens": maxTokens,
		"stream":     stream,
		"messages": []any{
			map[string]any{"role": "user", "content": userContent},
		},
	}
	if hasCell(cells, cellSystem) {
		m["system"] = systemText
	}
	if hasCell(cells, cellTools) {
		m["tools"] = []any{map[string]any{
			"name":         "get_time",
			"description":  "Current time",
			"input_schema": map[string]any{"type": "object", "properties": map[string]any{}},
		}}
	}
	return json.Marshal(m)
}

func geminiBundle(cells []cell) ([]byte, error) {
	parts := make([]any, 0, 6)
	if hasCell(cells, cellImage) {
		parts = append(parts, inlineData("image/png", png16))
	}
	if hasCell(cells, cellDocument) {
		parts = append(parts, inlineData("application/pdf", miniPDF()))
	}
	if hasCell(cells, cellAudio) {
		parts = append(parts, inlineData("audio/wav", wavSilence()))
	}
	if hasCell(cells, cellVideo) {
		parts = append(parts, inlineData("video/mp4", miniMP4()))
	}
	parts = append(parts, map[string]any{"text": userText})
	m := map[string]any{
		"generationConfig": map[string]any{"maxOutputTokens": maxTokens},
		"contents":         []any{map[string]any{"role": "user", "parts": parts}},
	}
	if hasCell(cells, cellSystem) {
		m["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": systemText}}}
	}
	if hasCell(cells, cellTools) {
		m["tools"] = []any{map[string]any{"functionDeclarations": []any{map[string]any{
			"name":        "get_time",
			"description": "Current time",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		}}}}
	}
	return json.Marshal(m)
}

func chatTool() map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "get_time",
			"description": "Current time",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}
}

func inlineData(mime string, raw []byte) map[string]any {
	return map[string]any{"inlineData": map[string]any{"mimeType": mime, "data": b64(raw)}}
}

func dataURL(mime string, raw []byte) string {
	return "data:" + mime + ";base64," + b64(raw)
}

func b64(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}
