package livetest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/thomas-huang/caosi/internal/config"
)

func parseClientResponse(proto config.Protocol, stream bool, body []byte) error {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return fmt.Errorf("empty body")
	}
	if stream {
		if err := parseStream(proto, body); err == nil {
			return nil
		}
		if err := parseJSONBody(proto, body); err == nil {
			return nil
		}
		return fmt.Errorf("neither legal SSE nor JSON for %s", proto)
	}
	return parseJSONBody(proto, body)
}

func parseJSONBody(proto config.Protocol, body []byte) error {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return fmt.Errorf("json: %w", err)
	}
	if !looksLike(proto, m) {
		return fmt.Errorf("JSON is not a %s body", proto)
	}
	return nil
}

func parseStream(proto config.Protocol, body []byte) error {
	payloads := sseDataPayloads(body)
	if len(payloads) == 0 {
		payloads = ndjsonPayloads(body)
	}
	ok := 0
	for _, p := range payloads {
		if bytes.Equal(p, []byte("[DONE]")) {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(p, &m); err != nil {
			continue
		}
		if looksLike(proto, m) {
			ok++
		}
	}
	if ok == 0 {
		return fmt.Errorf("no %s payload in stream", proto)
	}
	return nil
}

func sseDataPayloads(body []byte) [][]byte {
	var out [][]byte
	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		p := bytes.TrimSpace(line[5:])
		if len(p) == 0 {
			continue
		}
		out = append(out, p)
	}
	return out
}

func ndjsonPayloads(body []byte) [][]byte {
	dec := json.NewDecoder(bytes.NewReader(body))
	var out [][]byte
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return out
		}
		out = append(out, bytes.TrimSpace(raw))
	}
	return out
}

func looksLike(proto config.Protocol, m map[string]any) bool {
	if m == nil {
		return false
	}
	if errObj, ok := m["error"]; ok && errObj != nil {
		return true
	}
	switch proto {
	case config.ProtocolOpenAIChat:
		_, ok := m["choices"]
		return ok
	case config.ProtocolOpenAIResponses:
		if _, ok := m["output"]; ok {
			return true
		}
		if _, ok := m["status"]; ok {
			return true
		}
		if m["object"] == "response" {
			return true
		}
		t, _ := m["type"].(string)
		return t != "" && (strings.HasPrefix(t, "response") || strings.Contains(t, "error"))
	case config.ProtocolClaudeMessages:
		t, _ := m["type"].(string)
		switch t {
		case "message", "error", "message_start", "message_delta", "message_stop",
			"content_block_start", "content_block_delta", "content_block_stop", "ping":
			return true
		}
		_, hasContent := m["content"]
		return t == "" && hasContent
	case config.ProtocolGemini:
		_, ok := m["candidates"]
		return ok
	}
	return false
}
