package convert

import (
	"bytes"
	"strings"
	"testing"
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
