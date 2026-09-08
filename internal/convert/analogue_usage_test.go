package convert

import (
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestResponse_UsageDetailsAnalogue(t *testing.T) {
	chat := []byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"pong-p1"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":13},"completion_tokens_details":{"reasoning_tokens":17}}}`)
	responses := []byte(`{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong-p1"}]}],"usage":{"input_tokens":10,"output_tokens":5,"input_tokens_details":{"cached_tokens":13},"output_tokens_details":{"reasoning_tokens":17}}}`)
	claude := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"pong-p1"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":13,"cache_creation_input_tokens":11}}`)
	gemini := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"pong-p1"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"thoughtsTokenCount":17,"cachedContentTokenCount":13}}`)

	check := func(t *testing.T, upstream config.Protocol, body []byte, hasReasoning, hasCreate bool) {
		t.Helper()
		for _, client := range allProtocols {
			if client == upstream {
				continue
			}
			t.Run(string(upstream)+"→"+string(client), func(t *testing.T) {
				s := p1Resp(t, client, upstream, body)
				switch client {
				case config.ProtocolOpenAIChat:
					mustHave(t, s, `"cached_tokens":13`)
					if hasReasoning {
						mustHave(t, s, `"reasoning_tokens":17`)
					} else {
						mustOmit(t, s, `"reasoning_tokens":17`)
					}
					mustOmit(t, s, "cache_creation", "cache_create")
				case config.ProtocolOpenAIResponses:
					mustHave(t, s, `"cached_tokens":13`)
					if hasReasoning {
						mustHave(t, s, `"reasoning_tokens":17`)
					} else {
						mustOmit(t, s, `"reasoning_tokens":17`)
					}
					mustOmit(t, s, "cache_creation")
				case config.ProtocolClaudeMessages:
					mustHave(t, s, `"cache_read_input_tokens":13`)
					mustOmit(t, s, "reasoning_tokens", "thoughtsTokenCount")
					if hasCreate {
						mustHave(t, s, `"cache_creation_input_tokens":11`)
					} else {
						mustOmit(t, s, `"cache_creation_input_tokens"`)
					}
				case config.ProtocolGemini:
					mustHave(t, s, `"cachedContentTokenCount":13`)
					if hasReasoning {
						mustHave(t, s, `"thoughtsTokenCount":17`)
					} else {
						mustOmit(t, s, `"thoughtsTokenCount":17`)
					}
					mustOmit(t, s, "cache_creation")
				}
			})
		}
	}

	t.Run("chat", func(t *testing.T) { check(t, config.ProtocolOpenAIChat, chat, true, false) })
	t.Run("responses", func(t *testing.T) { check(t, config.ProtocolOpenAIResponses, responses, true, false) })
	t.Run("claude", func(t *testing.T) { check(t, config.ProtocolClaudeMessages, claude, false, true) })
	t.Run("gemini", func(t *testing.T) { check(t, config.ProtocolGemini, gemini, true, false) })
}

func TestResponse_UsageDetails_ClaudeCreateDroppedOnOthers(t *testing.T) {
	claude := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"pong-p1"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1,"cache_creation_input_tokens":11}}`)
	for _, client := range []config.Protocol{config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, config.ProtocolGemini} {
		s := p1Resp(t, client, config.ProtocolClaudeMessages, claude)
		t.Run(string(client), func(t *testing.T) {
			mustOmit(t, s, `"cache_creation_input_tokens":11`, `"cached_tokens":11`)
			if !strings.Contains(s, "pong-p1") {
				t.Fatalf("text dropped:\n%s", s)
			}
		})
	}
}
