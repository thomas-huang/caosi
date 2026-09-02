package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"caosi/internal/config"
)

func Supported(client, upstream config.Protocol) bool {
	return client.Valid() && upstream.Valid()
}

func NeedsConvert(client, upstream config.Protocol) bool {
	return client != upstream
}

func UpstreamPath(client, upstream config.Protocol, clientPath, model string, stream bool) string {
	if client == upstream {
		return clientPath
	}
	switch upstream {
	case config.ProtocolOpenAIChat:
		return "/v1/chat/completions"
	case config.ProtocolOpenAIResponses:
		return "/v1/responses"
	case config.ProtocolClaudeMessages:
		return "/v1/messages"
	case config.ProtocolGemini:
		if model == "" {
			model = "gemini-pro"
		}
		action := "generateContent"
		if stream {
			action = "streamGenerateContent"
		}
		return "/v1beta/models/" + model + ":" + action
	}
	return clientPath
}

func UnsupportedMessage(client, upstream config.Protocol) string {
	return fmt.Sprintf("无法把 %s 转成 %s", client, upstream)
}

func Request(client, upstream config.Protocol, body []byte, model string, stream bool) ([]byte, error) {
	if client == upstream {
		return applyModel(body, model)
	}
	// First-party cells CPA lacks: Claude/Gemini → Responses.
	if client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIResponses {
		return ClaudeToResponses(body, model, stream)
	}
	if client == config.ProtocolGemini && upstream == config.ProtocolOpenAIResponses {
		return GeminiToResponses(body, model, stream)
	}
	chat, err := toChatRequest(client, body, model, stream)
	if err != nil {
		return nil, err
	}
	return fromChatRequest(upstream, chat, model, stream)
}

func Response(client, upstream config.Protocol, body []byte) ([]byte, error) {
	if client == upstream {
		return body, nil
	}
	if looksLikeError(body) {
		return translateError(client, body)
	}
	chat, err := toChatResponse(upstream, body)
	if err != nil {
		return nil, err
	}
	return fromChatResponse(client, chat)
}

func Stream(client, upstream config.Protocol, r io.Reader, w io.Writer) error {
	if client == upstream {
		_, err := io.Copy(w, r)
		return err
	}
	if client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIChat {
		return OpenAIChatStreamToClaude(r, w)
	}
	if client == config.ProtocolOpenAIResponses && upstream == config.ProtocolOpenAIChat {
		return OpenAIChatStreamToResponses(r, w)
	}
	if client == config.ProtocolGemini && upstream == config.ProtocolOpenAIChat {
		return OpenAIChatStreamToGemini(r, w)
	}
	// Remaining stream pairs: buffer upstream SSE/JSON, convert as a full response, emit client SSE or JSON.
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	payload := assembleUpstreamResponse(raw)
	out, err := Response(client, upstream, payload)
	if err != nil {
		return err
	}
	if client == config.ProtocolClaudeMessages {
		return writeClaudeOneShotSSE(w, out)
	}
	if client == config.ProtocolOpenAIResponses {
		return writeResponsesOneShotSSE(w, out)
	}
	if client == config.ProtocolGemini {
		_, err := w.Write(out)
		return err
	}
	// OpenAI Chat client: wrap as SSE
	fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", out)
	return nil
}

func toChatRequest(p config.Protocol, body []byte, model string, stream bool) ([]byte, error) {
	switch p {
	case config.ProtocolOpenAIChat:
		return applyModel(body, model)
	case config.ProtocolOpenAIResponses:
		return ResponsesToChat(body, model, stream)
	case config.ProtocolClaudeMessages:
		return ClaudeToOpenAIChat(body, model, stream)
	case config.ProtocolGemini:
		return GeminiToOpenAIChat(body, model, stream)
	default:
		return nil, fmt.Errorf("未知 Client Protocol %s", p)
	}
}

func fromChatRequest(p config.Protocol, chat []byte, model string, stream bool) ([]byte, error) {
	switch p {
	case config.ProtocolOpenAIChat:
		return applyModel(chat, model)
	case config.ProtocolOpenAIResponses:
		return ChatToResponses(chat, model, stream)
	case config.ProtocolClaudeMessages:
		return OpenAIChatToClaudeRequest(chat, model, stream)
	case config.ProtocolGemini:
		return OpenAIChatToGemini(chat, model, stream)
	default:
		return nil, fmt.Errorf("未知 Upstream Protocol %s", p)
	}
}

func toChatResponse(p config.Protocol, body []byte) ([]byte, error) {
	switch p {
	case config.ProtocolOpenAIChat:
		return body, nil
	case config.ProtocolOpenAIResponses:
		return ResponsesToChatResponse(body)
	case config.ProtocolClaudeMessages:
		return ClaudeToChatResponse(body)
	case config.ProtocolGemini:
		return GeminiToChatResponse(body)
	default:
		return nil, fmt.Errorf("未知 Upstream Protocol %s", p)
	}
}

func fromChatResponse(p config.Protocol, chat []byte) ([]byte, error) {
	switch p {
	case config.ProtocolOpenAIChat:
		return chat, nil
	case config.ProtocolOpenAIResponses:
		return ChatToResponsesResponse(chat)
	case config.ProtocolClaudeMessages:
		return OpenAIChatToClaude(chat)
	case config.ProtocolGemini:
		return ChatToGeminiResponse(chat)
	default:
		return nil, fmt.Errorf("未知 Client Protocol %s", p)
	}
}

func applyModel(body []byte, model string) ([]byte, error) {
	if model == "" || len(bytes.TrimSpace(body)) == 0 {
		return body, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return body, nil
	}
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	m["model"] = b
	return json.Marshal(m)
}

func ModelFromBody(body []byte) string {
	var m struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &m)
	return strings.TrimSpace(m.Model)
}

func looksLikeError(body []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return false
	}
	raw, ok := m["error"]
	if !ok {
		return false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var errObj struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &errObj) != nil {
		return true
	}
	return strings.TrimSpace(errObj.Message) != ""
}

func translateError(client config.Protocol, body []byte) ([]byte, error) {
	msg := "upstream error"
	typ := "api_error"
	var wrap struct {
		Error *struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Status  string `json:"status"`
		} `json:"error"`
		Type string `json:"type"`
	}
	_ = json.Unmarshal(body, &wrap)
	if wrap.Error != nil && wrap.Error.Message != "" {
		msg = wrap.Error.Message
		if wrap.Error.Type != "" {
			typ = wrap.Error.Type
		}
	}
	_, b, _ := ClientError(client, 400, msg)
	if client == config.ProtocolClaudeMessages {
		return ClaudeError(typ, msg)
	}
	return b, nil
}

func assembleUpstreamResponse(raw []byte) []byte {
	s := bytes.TrimSpace(raw)
	if bytes.HasPrefix(s, []byte("{")) {
		return unwrapEventEnvelope(s)
	}
	var (
		last           []byte
		claudeText     strings.Builder
		claudeThinking strings.Builder
		chatText       strings.Builder
		chatThink      strings.Builder
		best           []byte
	)
	for _, line := range bytes.Split(s, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("event:")) {
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[5:])
		}
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		if !bytes.HasPrefix(line, []byte("{")) {
			continue
		}
		last = append([]byte(nil), line...)
		var m map[string]json.RawMessage
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		typ := strings.Trim(string(m["type"]), `"`)
		switch typ {
		case "response.completed", "response.incomplete":
			if r, ok := m["response"]; ok && len(bytes.TrimSpace(r)) > 0 && !bytes.Equal(bytes.TrimSpace(r), []byte("null")) {
				best = append([]byte(nil), r...)
			} else {
				best = unwrapEventEnvelope(line)
			}
		case "message":
			if _, ok := m["content"]; ok {
				best = append([]byte(nil), line...)
			}
		case "message_start":
			if msg, ok := m["message"]; ok {
				best = append([]byte(nil), msg...)
			}
		case "content_block_delta":
			var delta map[string]any
			if json.Unmarshal(m["delta"], &delta) == nil {
				if t, ok := delta["text"].(string); ok {
					claudeText.WriteString(t)
				}
				if t, ok := delta["thinking"].(string); ok {
					claudeThinking.WriteString(t)
				}
			}
		}
		if bytes.Contains(m["object"], []byte(`"response"`)) && m["output"] != nil {
			best = unwrapEventEnvelope(line)
		}
		if choices, ok := m["choices"]; ok {
			var chs []map[string]json.RawMessage
			if json.Unmarshal(choices, &chs) == nil && len(chs) > 0 {
				var delta map[string]json.RawMessage
				src := chs[0]["delta"]
				if len(src) == 0 {
					src = chs[0]["message"]
				}
				if json.Unmarshal(src, &delta) == nil {
					var t, th string
					_ = json.Unmarshal(delta["content"], &t)
					_ = json.Unmarshal(delta["reasoning_content"], &th)
					chatText.WriteString(t)
					chatThink.WriteString(th)
				}
			}
		}
	}
	if claudeText.Len() > 0 || claudeThinking.Len() > 0 {
		msg := claudeResp{ID: "msg_caosi", Type: "message", Role: "assistant", StopReason: "end_turn"}
		if claudeThinking.Len() > 0 {
			msg.Content = append(msg.Content, claudeBlock{Type: "thinking", Thinking: claudeThinking.String()})
		}
		if claudeText.Len() > 0 {
			msg.Content = append(msg.Content, claudeBlock{Type: "text", Text: claudeText.String()})
		}
		b, _ := json.Marshal(msg)
		return b
	}
	if len(best) > 0 {
		return unwrapEventEnvelope(best)
	}
	if chatText.Len() > 0 || chatThink.Len() > 0 {
		b, _ := json.Marshal(map[string]any{
			"id": "chatcmpl_caosi",
			"choices": []any{map[string]any{
				"message": map[string]any{
					"role":              "assistant",
					"content":           chatText.String(),
					"reasoning_content": chatThink.String(),
				},
				"finish_reason": "stop",
			}},
		})
		return b
	}
	if last != nil {
		return unwrapEventEnvelope(last)
	}
	return raw
}

func unwrapEventEnvelope(raw []byte) []byte {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return raw
	}
	typ := strings.Trim(string(m["type"]), `"`)
	if strings.HasPrefix(typ, "response.") {
		if r, ok := m["response"]; ok {
			r = bytes.TrimSpace(r)
			if len(r) > 0 && !bytes.Equal(r, []byte("null")) {
				return r
			}
		}
	}
	return raw
}

func writeClaudeOneShotSSE(w io.Writer, messageJSON []byte) error {
	var msg map[string]any
	if err := json.Unmarshal(messageJSON, &msg); err != nil {
		return err
	}
	start, _ := json.Marshal(map[string]any{"type": "message_start", "message": msg})
	if err := writeSSE(w, "message_start", start); err != nil {
		return err
	}
	stop, _ := json.Marshal(map[string]any{"type": "message_stop"})
	return writeSSE(w, "message_stop", stop)
}

func writeResponsesOneShotSSE(w io.Writer, respJSON []byte) error {
	fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", respJSON)
	return nil
}
