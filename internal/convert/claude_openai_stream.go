package convert

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// openAIChatStreamToClaude reads OpenAI Chat Completions SSE and writes Claude Messages SSE.
func openAIChatStreamToClaude(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	st := &claudeStreamState{id: "msg_caosi"}
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[5:])
		}
		if bytes.Equal(line, []byte("[DONE]")) {
			break
		}
		if err := st.feed(line, w); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return st.finish(w)
}

type claudeStreamState struct {
	id            string
	model         string
	started       bool
	textStarted   bool
	thinkingStart bool
	textIdx       int
	thinkIdx      int
	stopped       bool
	stopReason    string
	usage         claudeUsage
	toolIndex     map[int]int
	nextIndex     int
}

func (s *claudeStreamState) feed(raw []byte, w io.Writer) error {
	var chunk openaiChatResp
	if err := json.Unmarshal(raw, &chunk); err != nil {
		return nil
	}
	if chunk.Error != nil && chunk.Error.Message != "" {
		b, _ := encodeClaudeError(chunk.Error.Type, chunk.Error.Message)
		return writeSSE(w, "error", b)
	}
	if chunk.ID != "" {
		s.id = chunk.ID
	}
	if chunk.Model != "" {
		s.model = chunk.Model
	}
	if chunk.Usage != nil {
		s.usage.InputTokens = chunk.Usage.PromptTokens
		s.usage.OutputTokens = chunk.Usage.CompletionTokens
	}
	if !s.started {
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
				"usage":       map[string]int{"input_tokens": 0, "output_tokens": 0},
			},
		}
		b, _ := json.Marshal(msg)
		if err := writeSSE(w, "message_start", b); err != nil {
			return err
		}
	}
	if len(chunk.Choices) == 0 {
		return nil
	}
	ch := chunk.Choices[0]
	if ch.FinishReason != "" {
		s.stopReason = mapFinishReason(ch.FinishReason)
	}

	deltaContent := ""
	deltaThink := ""
	var toolDeltas []struct {
		Index    int
		ID       string
		Name     string
		ArgsFrag string
	}

	var generic map[string]json.RawMessage
	if err := json.Unmarshal(raw, &generic); err == nil {
		if choices, ok := generic["choices"]; ok {
			var chs []map[string]json.RawMessage
			if json.Unmarshal(choices, &chs) == nil && len(chs) > 0 {
				var delta map[string]json.RawMessage
				if json.Unmarshal(chs[0]["delta"], &delta) == nil {
					if c, ok := delta["content"]; ok {
						_ = json.Unmarshal(c, &deltaContent)
					}
					if c, ok := delta["reasoning_content"]; ok {
						_ = json.Unmarshal(c, &deltaThink)
					}
					if tcs, ok := delta["tool_calls"]; ok {
						var arr []map[string]json.RawMessage
						if json.Unmarshal(tcs, &arr) == nil {
							for _, tc := range arr {
								var idx int
								_ = json.Unmarshal(tc["index"], &idx)
								var id, typ string
								_ = json.Unmarshal(tc["id"], &id)
								var fn map[string]string
								_ = json.Unmarshal(tc["function"], &fn)
								toolDeltas = append(toolDeltas, struct {
									Index    int
									ID       string
									Name     string
									ArgsFrag string
								}{idx, id, fn["name"], fn["arguments"]})
								_ = typ
							}
						}
					}
				}
			}
		}
	}

	if deltaThink != "" {
		if !s.thinkingStart {
			s.thinkingStart = true
			s.thinkIdx = s.nextIndex
			s.nextIndex++
			b, _ := json.Marshal(map[string]any{
				"type":          "content_block_start",
				"index":         s.thinkIdx,
				"content_block": map[string]any{"type": "thinking", "thinking": ""},
			})
			if err := writeSSE(w, "content_block_start", b); err != nil {
				return err
			}
		}
		b, _ := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"index": s.thinkIdx,
			"delta": map[string]any{"type": "thinking_delta", "thinking": deltaThink},
		})
		if err := writeSSE(w, "content_block_delta", b); err != nil {
			return err
		}
	}

	if deltaContent != "" {
		if !s.textStarted {
			s.textStarted = true
			s.textIdx = s.nextIndex
			s.nextIndex++
			b, _ := json.Marshal(map[string]any{
				"type":          "content_block_start",
				"index":         s.textIdx,
				"content_block": map[string]any{"type": "text", "text": ""},
			})
			if err := writeSSE(w, "content_block_start", b); err != nil {
				return err
			}
		}
		b, _ := json.Marshal(map[string]any{
			"type":  "content_block_delta",
			"index": s.textIdx,
			"delta": map[string]any{"type": "text_delta", "text": deltaContent},
		})
		if err := writeSSE(w, "content_block_delta", b); err != nil {
			return err
		}
	}

	for _, td := range toolDeltas {
		if s.toolIndex == nil {
			s.toolIndex = map[int]int{}
		}
		idx, ok := s.toolIndex[td.Index]
		if !ok {
			idx = s.nextIndex
			s.nextIndex++
			s.toolIndex[td.Index] = idx
			b, _ := json.Marshal(map[string]any{
				"type":  "content_block_start",
				"index": idx,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    td.ID,
					"name":  td.Name,
					"input": map[string]any{},
				},
			})
			if err := writeSSE(w, "content_block_start", b); err != nil {
				return err
			}
		}
		if td.ArgsFrag != "" {
			b, _ := json.Marshal(map[string]any{
				"type":  "content_block_delta",
				"index": idx,
				"delta": map[string]any{"type": "input_json_delta", "partial_json": td.ArgsFrag},
			})
			if err := writeSSE(w, "content_block_delta", b); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *claudeStreamState) finish(w io.Writer) error {
	if s.stopped {
		return nil
	}
	s.stopped = true
	if !s.started {
		return nil
	}
	for i := 0; i < s.nextIndex; i++ {
		b, _ := json.Marshal(map[string]any{"type": "content_block_stop", "index": i})
		if err := writeSSE(w, "content_block_stop", b); err != nil {
			return err
		}
	}
	if s.stopReason == "" {
		s.stopReason = "end_turn"
	}
	b, _ := json.Marshal(map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": s.stopReason, "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": s.usage.OutputTokens},
	})
	if err := writeSSE(w, "message_delta", b); err != nil {
		return err
	}
	b, _ = json.Marshal(map[string]any{"type": "message_stop"})
	return writeSSE(w, "message_stop", b)
}

func writeSSE(w io.Writer, event string, data []byte) error {
	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return err
	}
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
	return nil
}
