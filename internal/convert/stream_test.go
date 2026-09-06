package convert

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestStream_ChatToClaude_TextChunks(t *testing.T) {
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
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
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

func TestStream_ClaudeFromGeminiJSON_ThinkingTextAndTool(t *testing.T) {
	in := `{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"hmm"},{"text":"ok"},{"functionCall":{"name":"read_file","args":{"path":"README.md"}}}]},"finishReason":"STOP"}]}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolGemini, strings.NewReader(in), &out); err != nil {
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

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitContains(t *testing.T, buf *syncBuf, sub string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), sub) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q, got:\n%s", sub, buf.String())
}

func TestStream_ClaudeFromResponses_TextDeltaBeforeCompleted(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	out := &syncBuf{}
	done := make(chan error, 1)
	go func() {
		done <- Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, pr, out)
	}()

	if _, err := io.WriteString(pw, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hel\"}\n\n"); err != nil {
		t.Fatal(err)
	}
	waitContains(t, out, `"type":"text_delta"`)
	waitContains(t, out, "hel")
	if strings.Contains(out.String(), "message_stop") {
		t.Fatal("emitted message_stop before response.completed")
	}

	if _, err := io.WriteString(pw, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\"}}\n\n"); err != nil {
		t.Fatal(err)
	}
	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after response.completed")
	}
	if !strings.Contains(out.String(), "event: message_stop") {
		t.Fatalf("missing message_stop:\n%s", out.String())
	}
}

func TestStream_ClaudeFromResponses_ThinkingAndToolBeforeCompleted(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	out := &syncBuf{}
	done := make(chan error, 1)
	go func() {
		done <- Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, pr, out)
	}()

	if _, err := io.WriteString(pw, "event: response.reasoning_summary_text.delta\ndata: {\"type\":\"response.reasoning_summary_text.delta\",\"delta\":\"hmm\"}\n\n"); err != nil {
		t.Fatal(err)
	}
	waitContains(t, out, `"type":"thinking_delta"`)
	waitContains(t, out, "hmm")

	if _, err := io.WriteString(pw, "event: response.output_item.added\ndata: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"fc_1\",\"type\":\"function_call\",\"name\":\"read_file\",\"call_id\":\"call_1\",\"arguments\":\"\"}}\n\n"); err != nil {
		t.Fatal(err)
	}
	waitContains(t, out, `"type":"tool_use"`)
	waitContains(t, out, "read_file")

	if _, err := io.WriteString(pw, "event: response.function_call_arguments.delta\ndata: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_1\",\"delta\":\"{\\\"path\\\":\\\"README.md\\\"}\"}\n\n"); err != nil {
		t.Fatal(err)
	}
	waitContains(t, out, `"type":"input_json_delta"`)
	waitContains(t, out, "README.md")
	if strings.Contains(out.String(), "message_stop") {
		t.Fatal("emitted message_stop before response.completed")
	}

	if _, err := io.WriteString(pw, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\"}}\n\n"); err != nil {
		t.Fatal(err)
	}
	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after response.completed")
	}
}

func TestStream_ClaudeFromResponses_CompletedSnapshotWithoutDeltas(t *testing.T) {
	in := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]},{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}]}]}}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, `"type":"text_delta"`) || !strings.Contains(s, "ok") {
		t.Fatalf("snapshot text dropped:\n%s", s)
	}
	if !strings.Contains(s, `"type":"thinking_delta"`) || !strings.Contains(s, "hmm") {
		t.Fatalf("snapshot thinking dropped:\n%s", s)
	}
	if !strings.Contains(s, "event: message_stop") {
		t.Fatalf("missing message_stop:\n%s", s)
	}
}

func TestStream_ClaudeFromResponses_ErrorJSON(t *testing.T) {
	in := `{"error":{"message":"quota","type":"insufficient_quota"}}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "event: error") || !strings.Contains(s, "quota") {
		t.Fatalf("want Claude error SSE, got\n%s", s)
	}
	if strings.Contains(s, "event: message_start") {
		t.Fatalf("error became success stream:\n%s", s)
	}
}

func TestStream_ClaudeFromResponses_ReasoningItemAdded(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	out := &syncBuf{}
	done := make(chan error, 1)
	go func() {
		done <- Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, pr, out)
	}()
	if _, err := io.WriteString(pw, `event: response.output_item.added
data: {"type":"response.output_item.added","item":{"id":"rs_1","type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}]}}

`); err != nil {
		t.Fatal(err)
	}
	waitContains(t, out, `"type":"thinking_delta"`)
	waitContains(t, out, "hmm")
	if strings.Contains(out.String(), "message_stop") {
		t.Fatal("emitted message_stop before response.completed")
	}
	if _, err := io.WriteString(pw, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\"}}\n\n"); err != nil {
		t.Fatal(err)
	}
	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after response.completed")
	}
}

func TestStream_ChatToGemini_EmitsGeminiJSON(t *testing.T) {
	in := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"reasoning_content":"hmm"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"hel"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolGemini, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, `"candidates"`) || !strings.Contains(s, "hello") {
		t.Fatalf("want Gemini stream JSON, got\n%s", s)
	}
	if !strings.Contains(s, "hmm") {
		t.Fatalf("thinking dropped:\n%s", s)
	}
	if strings.Contains(s, `"choices"`) {
		t.Fatalf("Chat schema leaked to Gemini client:\n%s", s)
	}
}

func TestStream_ResponsesOneShotSSE_FromClaudeJSON(t *testing.T) {
	in := `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn"}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "event: response.completed") {
		t.Fatalf("want one-shot Responses SSE, got\n%s", s)
	}
	if !strings.Contains(s, `"object":"response"`) || !strings.Contains(s, "pong") {
		t.Fatalf("want Responses body, got\n%s", s)
	}
	if strings.Contains(s, `"choices"`) || strings.Contains(s, `"type":"message"`) && !strings.Contains(s, `"object":"response"`) {
		t.Fatalf("upstream schema leaked:\n%s", s)
	}
}

func TestStream_ResponsesOneShotSSE_FromGeminiJSON(t *testing.T) {
	in := `{"candidates":[{"content":{"role":"model","parts":[{"text":"pong"}]},"finishReason":"STOP"}]}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIResponses, config.ProtocolGemini, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "event: response.completed") || !strings.Contains(s, "pong") {
		t.Fatalf("want Responses SSE, got\n%s", s)
	}
	if strings.Contains(s, `"candidates"`) || strings.Contains(s, `"choices"`) {
		t.Fatalf("upstream schema leaked:\n%s", s)
	}
}

func TestStream_ChatFromResponsesEnvelope(t *testing.T) {
	in := strings.Join([]string{
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}]}}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "data:") || !strings.Contains(s, "[DONE]") {
		t.Fatalf("want Chat SSE, got\n%s", s)
	}
	if !strings.Contains(s, `"choices"`) || !strings.Contains(s, "pong") {
		t.Fatalf("want Chat chunk, got\n%s", s)
	}
	if strings.Contains(s, `"object":"response"`) {
		t.Fatalf("Responses leaked to Chat client:\n%s", s)
	}
}

func TestStream_GeminiFromClaudeJSON(t *testing.T) {
	in := `{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"pong"}],"stop_reason":"end_turn"}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolGemini, config.ProtocolClaudeMessages, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, `"candidates"`) || !strings.Contains(s, "pong") {
		t.Fatalf("want Gemini JSON, got\n%s", s)
	}
	if strings.Contains(s, `"choices"`) || strings.Contains(s, `"type":"message"`) {
		t.Fatalf("Claude leaked to Gemini client:\n%s", s)
	}
}

func TestStream_ChatFromGeminiJSON(t *testing.T) {
	in := `{"candidates":[{"content":{"role":"model","parts":[{"text":"pong"}]},"finishReason":"STOP"}]}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIChat, config.ProtocolGemini, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, `"choices"`) || !strings.Contains(s, "pong") {
		t.Fatalf("want Chat SSE, got\n%s", s)
	}
	if !strings.Contains(s, "data: [DONE]") {
		t.Fatalf("want Chat SSE terminator:\n%s", s)
	}
	if strings.Contains(s, `"candidates"`) {
		t.Fatalf("Gemini leaked to Chat client:\n%s", s)
	}
}

func TestStream_PassthroughCopiesBytes(t *testing.T) {
	in := "data: raw-upstream\n\n"
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != in {
		t.Fatalf("passthrough mutated stream: %q", out.String())
	}
}

func TestStream_ClaudeFromGeminiJSON_LegalSSE(t *testing.T) {
	in := `{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"hmm"},{"text":"pong"}]},"finishReason":"STOP"}]}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolGemini, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "event: content_block_delta") || !strings.Contains(s, "pong") {
		t.Fatalf("want Claude SSE, got\n%s", s)
	}
	if strings.Contains(s, `"candidates"`) {
		t.Fatalf("Gemini leaked:\n%s", s)
	}
}

func TestStream_ChatToResponses_IncludesReasoning(t *testing.T) {
	in := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"reasoning_content":"think"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
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
