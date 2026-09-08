package convert

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func sseDataPayloads(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		p := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if p == "" || p == "[DONE]" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func countChatContentDeltas(payloads []string) int {
	n := 0
	for _, p := range payloads {
		var generic map[string]json.RawMessage
		if json.Unmarshal([]byte(p), &generic) != nil {
			continue
		}
		choices, ok := generic["choices"]
		if !ok {
			continue
		}
		var chs []map[string]json.RawMessage
		if json.Unmarshal(choices, &chs) != nil || len(chs) == 0 {
			continue
		}
		var delta map[string]json.RawMessage
		if json.Unmarshal(chs[0]["delta"], &delta) != nil {
			continue
		}
		var content, think string
		_ = json.Unmarshal(delta["content"], &content)
		_ = json.Unmarshal(delta["reasoning_content"], &think)
		if content != "" || think != "" || len(delta["tool_calls"]) > 0 {
			n++
		}
	}
	return n
}

func countResponsesContentDeltas(payloads []string) int {
	n := 0
	for _, p := range payloads {
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(p), &m) != nil {
			continue
		}
		typ := strings.Trim(string(m["type"]), `"`)
		if strings.HasSuffix(typ, ".delta") || strings.Contains(typ, "encrypted_content") {
			if eventDelta(m) != "" || rawString(m["encrypted_content"]) != "" {
				n++
			}
			continue
		}
		if typ == "response.output_item.added" {
			n++
		}
	}
	return n
}

func countClaudeContentDeltas(payloads []string) int {
	n := 0
	for _, p := range payloads {
		var m map[string]any
		if json.Unmarshal([]byte(p), &m) != nil {
			continue
		}
		if m["type"] == "content_block_delta" {
			n++
		}
	}
	return n
}

func countGeminiChunks(payloads []string) int {
	return len(payloads)
}

func TestStream_Inflation_FourIncrementalPairs(t *testing.T) {
	chatIn := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"role":"assistant","content":""}}]}`,
		``,
		`data: {"choices":[{"delta":{"reasoning_content":"hmm"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	chatPayloads := sseDataPayloads(chatIn)
	chatContent := countChatContentDeltas(chatPayloads)

	t.Run("claude←chat", func(t *testing.T) {
		var out bytes.Buffer
		if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(chatIn), &out); err != nil {
			t.Fatal(err)
		}
		got := countClaudeContentDeltas(sseDataPayloads(out.String()))
		if got > chatContent {
			t.Fatalf("content deltas inflated: in=%d out=%d\n%s", chatContent, got, out.String())
		}
	})
	t.Run("responses←chat", func(t *testing.T) {
		var out bytes.Buffer
		if err := Stream(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, strings.NewReader(chatIn), &out); err != nil {
			t.Fatal(err)
		}
		got := countResponsesContentDeltas(sseDataPayloads(out.String()))
		if got > chatContent {
			t.Fatalf("content deltas inflated: in=%d out=%d\n%s", chatContent, got, out.String())
		}
	})
	t.Run("gemini←chat", func(t *testing.T) {
		var out bytes.Buffer
		if err := Stream(config.ProtocolGemini, config.ProtocolOpenAIChat, strings.NewReader(chatIn), &out); err != nil {
			t.Fatal(err)
		}
		inChunks := countChatContentDeltas(chatPayloads)
		got := countGeminiChunks(sseDataPayloads(out.String()))
		if got > len(chatPayloads) {
			t.Fatalf("gemini chunks inflated: in data=%d out=%d\n%s", len(chatPayloads), got, out.String())
		}
		if inChunks == 0 {
			t.Fatal("expected chat content deltas")
		}
	})

	respIn := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.reasoning_summary_text.delta`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"hmm"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"hel"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"lo"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}`,
		``,
	}, "\n")
	t.Run("claude←responses", func(t *testing.T) {
		var out bytes.Buffer
		if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(respIn), &out); err != nil {
			t.Fatal(err)
		}
		inN := countResponsesContentDeltas(sseDataPayloads(respIn))
		got := countClaudeContentDeltas(sseDataPayloads(out.String()))
		if got > inN {
			t.Fatalf("content deltas inflated: in=%d out=%d\n%s", inN, got, out.String())
		}
	})
}
