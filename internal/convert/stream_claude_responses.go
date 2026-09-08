package convert

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/thomas-huang/caosi/internal/config"
)

// responsesStreamToClaude converts OpenAI Responses SSE into Claude Messages SSE
// as events arrive. A complete Responses JSON body (no SSE) still becomes a
// legal one-shot Claude stream.
func responsesStreamToClaude(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	st := &respToClaudeState{id: "msg_caosi"}
	var eventName string
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			eventName = ""
			continue
		}
		if bytes.HasPrefix(line, []byte("event:")) {
			eventName = string(bytes.TrimSpace(line[6:]))
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[5:])
		}
		if bytes.Equal(line, []byte("[DONE]")) {
			break
		}
		if err := st.feedLine(eventName, line, w); err != nil {
			return err
		}
		eventName = ""
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return st.finish(w)
}

type respToClaudeState struct {
	id            string
	model         string
	started       bool
	stopped       bool
	textStarted   bool
	thinkingStart bool
	textIdx       int
	thinkIdx      int
	nextIndex     int
	stopReason    string
	usage         claudeUsage
	toolIndex     map[string]int
	snapshot      json.RawMessage
}

func (s *respToClaudeState) feedLine(eventName string, line []byte, w io.Writer) error {
	var m map[string]json.RawMessage
	if json.Unmarshal(line, &m) != nil {
		return nil
	}
	typ := strings.Trim(string(m["type"]), `"`)
	if typ == "" {
		typ = eventName
	}
	if looksLikeError(line) {
		out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, line)
		if err != nil {
			return err
		}
		s.stopped = true
		return writeClaudeOneShotSSE(w, out)
	}
	if !s.started && looksLikeCompleteResponses(m) {
		out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, line)
		if err != nil {
			return err
		}
		s.stopped = true
		return writeClaudeOneShotSSE(w, out)
	}
	return s.feed(typ, m, w)
}

func looksLikeCompleteResponses(m map[string]json.RawMessage) bool {
	if m["output"] == nil {
		return false
	}
	obj := strings.Trim(string(m["object"]), `"`)
	status := strings.Trim(string(m["status"]), `"`)
	typ := strings.Trim(string(m["type"]), `"`)
	if strings.HasPrefix(typ, "response.") {
		return false
	}
	return obj == "response" || status == "completed" || status == "incomplete"
}

func (s *respToClaudeState) feed(typ string, m map[string]json.RawMessage, w io.Writer) error {
	if typ == "error" || typ == "response.failed" {
		s.stopped = true
		msg := "upstream error"
		errType := "api_error"
		if errObj := m["error"]; len(errObj) > 0 {
			var e struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			}
			_ = json.Unmarshal(errObj, &e)
			if e.Message != "" {
				msg = e.Message
			}
			if e.Type != "" {
				errType = e.Type
			}
		}
		b, _ := encodeClaudeError(errType, msg)
		return writeSSE(w, "error", b)
	}

	if nested, ok := m["response"]; ok {
		s.snapshot = nested
		var resp map[string]json.RawMessage
		if json.Unmarshal(nested, &resp) == nil {
			if id := rawString(resp["id"]); id != "" {
				s.id = id
			}
			if model := rawString(resp["model"]); model != "" {
				s.model = model
			}
			if u, ok := resp["usage"]; ok {
				var usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				}
				if json.Unmarshal(u, &usage) == nil {
					s.usage.InputTokens = usage.InputTokens
					s.usage.OutputTokens = usage.OutputTokens
				}
			}
			status := rawString(resp["status"])
			if status == "incomplete" {
				s.stopReason = "max_tokens"
			}
			if s.stopReason == "" && outputHasFunctionCall(resp["output"]) {
				s.stopReason = "tool_use"
			}
		}
	}

	switch {
	case typ == "response.completed" || typ == "response.incomplete":
		if typ == "response.incomplete" && s.stopReason == "" {
			s.stopReason = "max_tokens"
		}
		return s.applySnapshotIfNeeded(w)
	case typ == "response.output_text.delta":
		if err := s.ensureStart(w); err != nil {
			return err
		}
		return s.emitText(w, eventDelta(m))
	case typ == "response.reasoning_summary_text.delta" || typ == "response.reasoning_text.delta":
		if err := s.ensureStart(w); err != nil {
			return err
		}
		return s.emitThinking(w, eventDelta(m))
	case strings.Contains(typ, "encrypted_content"):
		if err := s.ensureStart(w); err != nil {
			return err
		}
		sig := eventDelta(m)
		if sig == "" {
			sig = rawString(m["encrypted_content"])
		}
		return s.emitSignature(w, sig)
	case typ == "response.output_item.added":
		if err := s.ensureStart(w); err != nil {
			return err
		}
		return s.handleItemAdded(m, w)
	case typ == "response.function_call_arguments.delta":
		if err := s.ensureStart(w); err != nil {
			return err
		}
		return s.emitToolArgs(w, rawString(m["item_id"]), eventDelta(m))
	default:
		if err := s.ensureStart(w); err != nil {
			return err
		}
		return nil
	}
}

func (s *respToClaudeState) applySnapshotIfNeeded(w io.Writer) error {
	if s.nextIndex > 0 || len(bytes.TrimSpace(s.snapshot)) == 0 {
		if s.nextIndex == 0 {
			return s.ensureStart(w)
		}
		return nil
	}
	out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, s.snapshot)
	if err != nil {
		return err
	}
	if looksLikeError(out) {
		s.stopped = true
		return writeSSE(w, "error", out)
	}
	var msg claudeResp
	if err := json.Unmarshal(out, &msg); err != nil {
		return err
	}
	if err := s.ensureStart(w); err != nil {
		return err
	}
	for _, block := range msg.Content {
		idx := s.nextIndex
		s.nextIndex++
		if err := writeClaudeOneShotBlock(w, idx, block); err != nil {
			return err
		}
		switch block.Type {
		case "thinking":
			s.thinkingStart = true
		case "text":
			s.textStarted = true
		case "tool_use":
			s.stopReason = "tool_use"
		}
	}
	if msg.StopReason != "" {
		s.stopReason = msg.StopReason
	}
	if msg.Usage.OutputTokens > 0 {
		s.usage = msg.Usage
	}
	return nil
}

func eventDelta(m map[string]json.RawMessage) string {
	if d := rawString(m["delta"]); d != "" {
		return d
	}
	return rawString(m["text"])
}

func outputHasFunctionCall(raw json.RawMessage) bool {
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return false
	}
	for _, it := range items {
		t := strings.Trim(string(it["type"]), `"`)
		if t == "function_call" || t == "custom_tool_call" {
			return true
		}
	}
	return false
}

func (s *respToClaudeState) ensureStart(w io.Writer) error {
	if s.started {
		return nil
	}
	s.started = true
	msg := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":          s.id,
			"type":        "message",
			"role":        "assistant",
			"model":       s.model,
			"content":     []any{},
			"stop_reason": nil,
			"usage":       map[string]int{"input_tokens": s.usage.InputTokens, "output_tokens": 0},
		},
	}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return writeSSE(w, "message_start", b)
}

func (s *respToClaudeState) ensureThinking(w io.Writer) error {
	if s.thinkingStart {
		return nil
	}
	s.thinkingStart = true
	s.thinkIdx = s.nextIndex
	s.nextIndex++
	b, err := json.Marshal(map[string]any{
		"type":          "content_block_start",
		"index":         s.thinkIdx,
		"content_block": map[string]any{"type": "thinking", "thinking": ""},
	})
	if err != nil {
		return err
	}
	return writeSSE(w, "content_block_start", b)
}

func (s *respToClaudeState) emitThinking(w io.Writer, delta string) error {
	if delta == "" {
		return nil
	}
	if err := s.ensureThinking(w); err != nil {
		return err
	}
	b, err := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": s.thinkIdx,
		"delta": map[string]any{"type": "thinking_delta", "thinking": delta},
	})
	if err != nil {
		return err
	}
	return writeSSE(w, "content_block_delta", b)
}

func (s *respToClaudeState) emitSignature(w io.Writer, sig string) error {
	if sig == "" {
		return nil
	}
	if err := s.ensureThinking(w); err != nil {
		return err
	}
	b, err := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": s.thinkIdx,
		"delta": map[string]any{"type": "signature_delta", "signature": sig},
	})
	if err != nil {
		return err
	}
	return writeSSE(w, "content_block_delta", b)
}

func (s *respToClaudeState) emitText(w io.Writer, delta string) error {
	if delta == "" {
		return nil
	}
	if !s.textStarted {
		s.textStarted = true
		s.textIdx = s.nextIndex
		s.nextIndex++
		b, err := json.Marshal(map[string]any{
			"type":          "content_block_start",
			"index":         s.textIdx,
			"content_block": map[string]any{"type": "text", "text": ""},
		})
		if err != nil {
			return err
		}
		if err := writeSSE(w, "content_block_start", b); err != nil {
			return err
		}
	}
	b, err := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": s.textIdx,
		"delta": map[string]any{"type": "text_delta", "text": delta},
	})
	if err != nil {
		return err
	}
	return writeSSE(w, "content_block_delta", b)
}

func (s *respToClaudeState) handleItemAdded(m map[string]json.RawMessage, w io.Writer) error {
	var item map[string]json.RawMessage
	if json.Unmarshal(m["item"], &item) != nil {
		return nil
	}
	typ := strings.Trim(string(item["type"]), `"`)
	if typ == "reasoning" {
		text := responsesOutputText(item["summary"]) + responsesOutputText(item["content"])
		if err := s.emitThinking(w, text); err != nil {
			return err
		}
		return s.emitSignature(w, rawString(item["encrypted_content"]))
	}
	if typ != "function_call" && typ != "custom_tool_call" {
		return nil
	}
	itemID := rawString(item["id"])
	callID := rawString(item["call_id"])
	id := firstNonEmpty(callID, itemID)
	name := rawString(item["name"])
	if err := s.startTool(w, id, name); err != nil {
		return err
	}
	s.aliasTool(itemID, id)
	s.aliasTool(callID, id)
	if args := rawString(item["arguments"]); args != "" {
		return s.emitToolArgs(w, id, args)
	}
	return nil
}

func (s *respToClaudeState) emitToolArgs(w io.Writer, itemID, delta string) error {
	if delta == "" {
		return nil
	}
	if err := s.startTool(w, itemID, ""); err != nil {
		return err
	}
	idx := s.toolIndex[itemID]
	b, err := json.Marshal(map[string]any{
		"type":  "content_block_delta",
		"index": idx,
		"delta": map[string]any{"type": "input_json_delta", "partial_json": delta},
	})
	if err != nil {
		return err
	}
	if s.stopReason == "" {
		s.stopReason = "tool_use"
	}
	return writeSSE(w, "content_block_delta", b)
}

func (s *respToClaudeState) aliasTool(alias, id string) {
	if alias == "" || id == "" || s.toolIndex == nil {
		return
	}
	if idx, ok := s.toolIndex[id]; ok {
		s.toolIndex[alias] = idx
	}
}

func (s *respToClaudeState) startTool(w io.Writer, id, name string) error {
	if s.toolIndex == nil {
		s.toolIndex = map[string]int{}
	}
	if id == "" {
		id = "toolu_caosi"
	}
	if _, ok := s.toolIndex[id]; ok {
		return nil
	}
	idx := s.nextIndex
	s.nextIndex++
	s.toolIndex[id] = idx
	if s.stopReason == "" {
		s.stopReason = "tool_use"
	}
	b, err := json.Marshal(map[string]any{
		"type":  "content_block_start",
		"index": idx,
		"content_block": map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  name,
			"input": map[string]any{},
		},
	})
	if err != nil {
		return err
	}
	return writeSSE(w, "content_block_start", b)
}

func (s *respToClaudeState) finish(w io.Writer) error {
	if s.stopped {
		return nil
	}
	s.stopped = true
	if !s.started {
		return nil
	}
	for i := 0; i < s.nextIndex; i++ {
		b, err := json.Marshal(map[string]any{"type": "content_block_stop", "index": i})
		if err != nil {
			return err
		}
		if err := writeSSE(w, "content_block_stop", b); err != nil {
			return err
		}
	}
	if s.stopReason == "" {
		s.stopReason = "end_turn"
	}
	b, err := json.Marshal(map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": s.stopReason, "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": s.usage.OutputTokens},
	})
	if err != nil {
		return err
	}
	if err := writeSSE(w, "message_delta", b); err != nil {
		return err
	}
	stop, err := json.Marshal(map[string]any{"type": "message_stop"})
	if err != nil {
		return err
	}
	return writeSSE(w, "message_stop", stop)
}
