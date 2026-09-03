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

func TestWriteClaudeOneShotSSE_LegalEventSequence(t *testing.T) {
	in := []byte(`{
	  "id":"msg_1","type":"message","role":"assistant","model":"glm-5.3",
	  "content":[
	    {"type":"thinking","thinking":"hmm"},
	    {"type":"text","text":"ok"},
	    {"type":"tool_use","id":"toolu_1","name":"read_file","input":{"path":"README.md"}}
	  ],
	  "stop_reason":"tool_use",
	  "usage":{"input_tokens":9,"output_tokens":4}
	}`)
	var out bytes.Buffer
	if err := writeClaudeOneShotSSE(&out, in); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"event: message_start",
		`"content":[]`,
		"event: content_block_start",
		"event: content_block_delta",
		`"type":"thinking_delta"`,
		"hmm",
		`"type":"text_delta"`,
		"ok",
		`"type":"input_json_delta"`,
		"read_file",
		"README.md",
		"event: content_block_stop",
		"event: message_delta",
		"tool_use",
		"event: message_stop",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}
	if strings.Contains(s, `"content":[{"type":"thinking"`) || strings.Contains(s, `"content":[{"type":"text"`) {
		t.Fatalf("message_start still stuffed with completed content:\n%s", s)
	}
}

func TestStream_ClaudeClientResponsesJSON_LegalSSE(t *testing.T) {
	in := `{"id":"resp_1","object":"response","status":"completed","error":null,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}]}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "event: content_block_delta") || !strings.Contains(s, `"type":"text_delta"`) {
		t.Fatalf("want legal Claude SSE, got\n%s", s)
	}
	if !strings.Contains(s, "ok") {
		t.Fatalf("lost text:\n%s", s)
	}
	if !strings.Contains(s, "event: message_delta") || !strings.Contains(s, "event: message_stop") {
		t.Fatalf("missing close events:\n%s", s)
	}
	if strings.Contains(s, `"content":[{"type":"text"`) {
		t.Fatalf("message_start stuffed with content:\n%s", s)
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
