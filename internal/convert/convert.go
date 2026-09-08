package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/thomas-huang/caosi/internal/config"
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
	ir, err := toIRRequest(client, body, model, stream)
	if err != nil {
		return nil, err
	}
	return fromIRRequest(upstream, ir, model, stream)
}

func Response(client, upstream config.Protocol, body []byte) ([]byte, error) {
	if client == upstream {
		return body, nil
	}
	if looksLikeError(body) {
		return translateError(client, body)
	}
	ir, err := toIRResponse(upstream, body)
	if err != nil {
		return nil, err
	}
	return fromIRResponse(client, ir)
}

func Stream(client, upstream config.Protocol, r io.Reader, w io.Writer) error {
	if client == upstream {
		_, err := io.Copy(w, r)
		return err
	}
	if client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIChat {
		return openAIChatStreamToClaude(r, w)
	}
	if client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIResponses {
		return responsesStreamToClaude(r, w)
	}
	if client == config.ProtocolOpenAIResponses && upstream == config.ProtocolOpenAIChat {
		return openAIChatStreamToResponses(r, w)
	}
	if client == config.ProtocolGemini && upstream == config.ProtocolOpenAIChat {
		return openAIChatStreamToGemini(r, w)
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

func toIRRequest(p config.Protocol, body []byte, model string, stream bool) (irRequest, error) {
	switch p {
	case config.ProtocolOpenAIChat:
		return chatToIRRequest(body, model, stream)
	case config.ProtocolOpenAIResponses:
		return responsesToIRRequest(body, model, stream)
	case config.ProtocolClaudeMessages:
		return claudeToIRRequest(body, model, stream)
	case config.ProtocolGemini:
		return geminiToIRRequest(body, model, stream)
	default:
		return irRequest{}, fmt.Errorf("未知 Client Protocol %s", p)
	}
}

func fromIRRequest(p config.Protocol, ir irRequest, model string, stream bool) ([]byte, error) {
	if model != "" {
		ir.Model = model
	}
	ir.Stream = stream
	switch p {
	case config.ProtocolOpenAIChat:
		return irToChatRequest(ir)
	case config.ProtocolOpenAIResponses:
		return irToResponsesRequest(ir)
	case config.ProtocolClaudeMessages:
		return irToClaudeRequest(ir)
	case config.ProtocolGemini:
		return irToGeminiRequest(ir)
	default:
		return nil, fmt.Errorf("未知 Upstream Protocol %s", p)
	}
}

func toIRResponse(p config.Protocol, body []byte) (irResponse, error) {
	switch p {
	case config.ProtocolOpenAIChat:
		return chatToIRResponse(body)
	case config.ProtocolOpenAIResponses:
		return responsesToIRResponse(body)
	case config.ProtocolClaudeMessages:
		return claudeToIRResponse(body)
	case config.ProtocolGemini:
		return geminiToIRResponse(body)
	default:
		return irResponse{}, fmt.Errorf("未知 Upstream Protocol %s", p)
	}
}

func fromIRResponse(p config.Protocol, ir irResponse) ([]byte, error) {
	switch p {
	case config.ProtocolOpenAIChat:
		return irToChatResponse(ir)
	case config.ProtocolOpenAIResponses:
		return irToResponsesResponse(ir)
	case config.ProtocolClaudeMessages:
		return irToClaudeResponse(ir)
	case config.ProtocolGemini:
		return irToGeminiResponse(ir)
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
		return encodeClaudeError(typ, msg)
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

// writeClaudeOneShotSSE turns a completed Claude message into a legal Messages SSE
// sequence. Claude Code ignores content on message_start and treats a stream
// without content_block_* as "ended before any complete data".
func writeClaudeOneShotSSE(w io.Writer, messageJSON []byte) error {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(messageJSON, &probe); err != nil {
		return err
	}
	if strings.Trim(string(probe["type"]), `"`) == "error" {
		return writeSSE(w, "error", messageJSON)
	}

	var msg claudeResp
	if err := json.Unmarshal(messageJSON, &msg); err != nil {
		return err
	}
	if msg.ID == "" {
		msg.ID = "msg_caosi"
	}
	if msg.Role == "" {
		msg.Role = "assistant"
	}

	start, err := json.Marshal(map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":          msg.ID,
			"type":        "message",
			"role":        msg.Role,
			"model":       msg.Model,
			"content":     []any{},
			"stop_reason": nil,
			"usage": map[string]int{
				"input_tokens":  msg.Usage.InputTokens,
				"output_tokens": 0,
			},
		},
	})
	if err != nil {
		return err
	}
	if err := writeSSE(w, "message_start", start); err != nil {
		return err
	}

	for i, block := range msg.Content {
		if err := writeClaudeOneShotBlock(w, i, block); err != nil {
			return err
		}
		stop, err := json.Marshal(map[string]any{"type": "content_block_stop", "index": i})
		if err != nil {
			return err
		}
		if err := writeSSE(w, "content_block_stop", stop); err != nil {
			return err
		}
	}

	stopReason := msg.StopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}
	delta, err := json.Marshal(map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{"output_tokens": msg.Usage.OutputTokens},
	})
	if err != nil {
		return err
	}
	if err := writeSSE(w, "message_delta", delta); err != nil {
		return err
	}
	stop, err := json.Marshal(map[string]any{"type": "message_stop"})
	if err != nil {
		return err
	}
	return writeSSE(w, "message_stop", stop)
}

func writeClaudeOneShotBlock(w io.Writer, index int, block claudeBlock) error {
	switch block.Type {
	case "thinking":
		start, err := json.Marshal(map[string]any{
			"type":          "content_block_start",
			"index":         index,
			"content_block": map[string]any{"type": "thinking", "thinking": ""},
		})
		if err != nil {
			return err
		}
		if err := writeSSE(w, "content_block_start", start); err != nil {
			return err
		}
		delta, err := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"index": index,
			"delta": map[string]any{"type": "thinking_delta", "thinking": block.Thinking},
		})
		if err != nil {
			return err
		}
		if err := writeSSE(w, "content_block_delta", delta); err != nil {
			return err
		}
		if block.Signature == "" {
			return nil
		}
		sig, err := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"index": index,
			"delta": map[string]any{"type": "signature_delta", "signature": block.Signature},
		})
		if err != nil {
			return err
		}
		return writeSSE(w, "content_block_delta", sig)
	case "tool_use":
		start, err := json.Marshal(map[string]any{
			"type":  "content_block_start",
			"index": index,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    block.ID,
				"name":  block.Name,
				"input": map[string]any{},
			},
		})
		if err != nil {
			return err
		}
		if err := writeSSE(w, "content_block_start", start); err != nil {
			return err
		}
		partial := "{}"
		if len(bytes.TrimSpace(block.Input)) > 0 {
			partial = string(block.Input)
		}
		delta, err := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"index": index,
			"delta": map[string]any{"type": "input_json_delta", "partial_json": partial},
		})
		if err != nil {
			return err
		}
		return writeSSE(w, "content_block_delta", delta)
	default:
		start, err := json.Marshal(map[string]any{
			"type":          "content_block_start",
			"index":         index,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
		if err != nil {
			return err
		}
		if err := writeSSE(w, "content_block_start", start); err != nil {
			return err
		}
		delta, err := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"index": index,
			"delta": map[string]any{"type": "text_delta", "text": block.Text},
		})
		if err != nil {
			return err
		}
		return writeSSE(w, "content_block_delta", delta)
	}
}

func writeResponsesOneShotSSE(w io.Writer, respJSON []byte) error {
	fmt.Fprintf(w, "event: response.completed\ndata: %s\n\n", respJSON)
	return nil
}
