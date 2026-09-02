package convert

import (
	"bytes"
	"strings"
	"testing"

	"caosi/internal/config"
)

func TestOpenAIChatStreamToClaude_TextChunks(t *testing.T) {
	in := strings.Join([]string{
		`data: {"id":"chatcmpl-1","model":"m","choices":[{"index":0,"delta":{"role":"assistant","content":"hel"}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{"content":"lo"}}]}`,
		``,
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := OpenAIChatStreamToClaude(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"event: message_start", "event: content_block_delta", "hel", "lo", "event: message_stop", "end_turn"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}
	if strings.Contains(s, `"choices"`) {
		t.Fatalf("OpenAI chunk leaked:\n%s", s)
	}
}

func TestStream_ClaudeUpstreamSSEAssemblesText(t *testing.T) {
	in := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[]}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "hello") {
		t.Fatalf("assembled stream lost text:\n%s", s)
	}
	if strings.Contains(s, "message_stop") && !strings.Contains(s, "hello") {
		t.Fatalf("used message_stop envelope:\n%s", s)
	}
}

func TestOpenAIChatStreamToResponses_IncludesReasoning(t *testing.T) {
	in := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"reasoning_content":"think"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := OpenAIChatStreamToResponses(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "response.completed") {
		t.Fatalf("missing completed:\n%s", s)
	}
	if !strings.Contains(s, "think") {
		t.Fatalf("reasoning omitted from SSE:\n%s", s)
	}
	if !strings.Contains(s, `"type":"reasoning"`) {
		t.Fatalf("want reasoning analogue:\n%s", s)
	}
}
