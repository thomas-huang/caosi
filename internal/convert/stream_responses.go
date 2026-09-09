package convert

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/thomas-huang/caosi/internal/config"
)

func openAIChatStreamToResponses(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	st := &respStreamState{id: "resp_caosi"}
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
		if st.stopped {
			return nil
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return st.finish(w)
}

type respStreamState struct {
	id        string
	model     string
	started   bool
	textOpen  bool
	stopped   bool
	text      string
	reasoning string
}

func (s *respStreamState) feed(raw []byte, w io.Writer) error {
	if looksLikeError(raw) {
		out, err := translateError(config.ProtocolOpenAIResponses, raw)
		if err != nil {
			return err
		}
		s.stopped = true
		return writeResponsesEvent(w, "error", out)
	}
	var generic map[string]json.RawMessage
	if json.Unmarshal(raw, &generic) != nil {
		return nil
	}
	if id := rawString(generic["id"]); id != "" {
		s.id = id
	}
	if m := rawString(generic["model"]); m != "" {
		s.model = m
	}
	if !s.started {
		s.started = true
		created, _ := json.Marshal(map[string]any{
			"type":     "response.created",
			"response": map[string]any{"id": s.id, "object": "response", "status": "in_progress", "model": s.model},
		})
		if err := writeResponsesEvent(w, "response.created", created); err != nil {
			return err
		}
	}
	deltaText, deltaThink := "", ""
	if choices, ok := generic["choices"]; ok {
		var chs []map[string]json.RawMessage
		if json.Unmarshal(choices, &chs) == nil && len(chs) > 0 {
			var delta map[string]json.RawMessage
			if json.Unmarshal(chs[0]["delta"], &delta) == nil {
				_ = json.Unmarshal(delta["content"], &deltaText)
				_ = json.Unmarshal(delta["reasoning_content"], &deltaThink)
			}
		}
	}
	if deltaThink != "" {
		s.reasoning += deltaThink
	}
	if deltaText != "" {
		s.text += deltaText
		ev, _ := json.Marshal(map[string]any{
			"type":  "response.output_text.delta",
			"delta": deltaText,
		})
		if err := writeResponsesEvent(w, "response.output_text.delta", ev); err != nil {
			return err
		}
	}
	return nil
}

func (s *respStreamState) finish(w io.Writer) error {
	if s.stopped {
		return nil
	}
	s.stopped = true
	var output []any
	if s.reasoning != "" {
		output = append(output, map[string]any{
			"type":    "reasoning",
			"summary": []any{map[string]any{"type": "summary_text", "text": s.reasoning}},
		})
	}
	output = append(output, map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": []any{map[string]any{"type": "output_text", "text": s.text}},
	})
	completed, _ := json.Marshal(map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":     s.id,
			"object": "response",
			"status": "completed",
			"model":  s.model,
			"output": output,
		},
	})
	return writeResponsesEvent(w, "response.completed", completed)
}

func writeResponsesEvent(w io.Writer, event string, data []byte) error {
	_, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
	return err
}

func openAIChatStreamToGemini(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	var text, think string
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(line[5:])
		}
		if len(line) == 0 || bytes.Equal(line, []byte("[DONE]")) {
			continue
		}
		var generic map[string]json.RawMessage
		if json.Unmarshal(line, &generic) != nil {
			continue
		}
		if looksLikeError(line) {
			out, err := translateError(config.ProtocolGemini, line)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(w, "data: %s\n\n", out)
			return err
		}
		if choices, ok := generic["choices"]; ok {
			var chs []map[string]json.RawMessage
			if json.Unmarshal(choices, &chs) == nil && len(chs) > 0 {
				var delta map[string]json.RawMessage
				if json.Unmarshal(chs[0]["delta"], &delta) == nil {
					var t, th string
					_ = json.Unmarshal(delta["content"], &t)
					_ = json.Unmarshal(delta["reasoning_content"], &th)
					text += t
					think += th
				}
			}
		}
		chunk, _ := chatToGeminiResponse([]byte(`{"choices":[{"message":{"role":"assistant","content":` + jsonString(text) + `,"reasoning_content":` + jsonString(think) + `},"finish_reason":"stop"}]}`))
		if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
			return err
		}
		if f, ok := w.(interface{ Flush() }); ok {
			f.Flush()
		}
	}
	return sc.Err()
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
