package convert

import (
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func rtRequest(p config.Protocol) []byte {
	switch p {
	case config.ProtocolOpenAIChat:
		return []byte(`{"model":"gpt-x","messages":[{"role":"system","content":"be brief"},{"role":"user","content":[{"type":"text","text":"hi-rt"},{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="}}]},{"role":"assistant","content":"pong-p1","reasoning_content":"hmm"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"w","parameters":{"type":"object"}}}]}`)
	case config.ProtocolOpenAIResponses:
		return []byte(`{"model":"gpt-x","instructions":"be brief","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi-rt"},{"type":"input_image","image_url":"data:image/png;base64,QQ=="}]},{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}],"encrypted_content":"sig-p2-carriage"}],"tools":[{"type":"function","name":"get_weather","description":"w","parameters":{"type":"object"}}]}`)
	case config.ProtocolClaudeMessages:
		return []byte(`{"model":"claude-opus","max_tokens":32,"system":"be brief","messages":[{"role":"user","content":[{"type":"text","text":"hi-rt"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QQ=="}}]},{"role":"assistant","content":[{"type":"thinking","thinking":"hmm","signature":"sig-p2-carriage"},{"type":"text","text":"pong-p1"}]}],"tools":[{"name":"get_weather","description":"w","input_schema":{"type":"object"}}]}`)
	case config.ProtocolGemini:
		return []byte(`{"systemInstruction":{"parts":[{"text":"be brief"}]},"contents":[{"role":"user","parts":[{"text":"hi-rt"},{"inlineData":{"mimeType":"image/png","data":"QQ=="}}]},{"role":"model","parts":[{"thought":true,"text":"hmm","thoughtSignature":"sig-p2-carriage"},{"text":"pong-p1"}]}],"tools":[{"functionDeclarations":[{"name":"get_weather","description":"w","parameters":{"type":"object"}}]}]}`)
	}
	return nil
}

func rtResponse(p config.Protocol) []byte {
	switch p {
	case config.ProtocolOpenAIChat:
		return []byte(`{"id":"chatcmpl-1","model":"m","choices":[{"message":{"role":"assistant","content":"pong-p1","reasoning_content":"hmm"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":13},"completion_tokens_details":{"reasoning_tokens":17}}}`)
	case config.ProtocolOpenAIResponses:
		return []byte(`{"id":"resp_1","object":"response","status":"completed","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}],"encrypted_content":"sig-p2-carriage"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong-p1"}]}],"usage":{"input_tokens":10,"output_tokens":5,"input_tokens_details":{"cached_tokens":13},"output_tokens_details":{"reasoning_tokens":17}}}`)
	case config.ProtocolClaudeMessages:
		return []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"hmm","signature":"sig-p2-carriage"},{"type":"text","text":"pong-p1"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":13,"cache_creation_input_tokens":11}}`)
	case config.ProtocolGemini:
		return []byte(`{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"hmm","thoughtSignature":"sig-p2-carriage"},{"text":"pong-p1"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"thoughtsTokenCount":17,"cachedContentTokenCount":13}}`)
	}
	return nil
}

func TestRequest_SemanticRoundTrip(t *testing.T) {
	for _, a := range allProtocols {
		for _, b := range allProtocols {
			if a == b {
				continue
			}
			t.Run(string(a)+"→"+string(b)+"→"+string(a), func(t *testing.T) {
				mid, err := Request(a, b, rtRequest(a), "m", false)
				if err != nil {
					t.Fatal(err)
				}
				back, err := Request(b, a, mid, "m", false)
				if err != nil {
					t.Fatal(err)
				}
				s := string(back)
				for _, want := range []string{"hi-rt", "be brief", "get_weather", "hmm", "QQ=="} {
					if !strings.Contains(s, want) {
						t.Fatalf("lost %q on %s→%s→%s:\n%s", want, a, b, a, s)
					}
				}
				if a != config.ProtocolOpenAIChat && b != config.ProtocolOpenAIChat {
					if !strings.Contains(s, p2Sig) {
						t.Fatalf("lost Thinking Signature on %s→%s→%s:\n%s", a, b, a, s)
					}
				} else if strings.Contains(s, p2Sig) && a == config.ProtocolOpenAIChat {
					t.Fatalf("Chat must not grow a Thinking Signature:\n%s", s)
				}
			})
		}
	}
}

func TestResponse_SemanticRoundTrip(t *testing.T) {
	for _, a := range allProtocols {
		for _, b := range allProtocols {
			if a == b {
				continue
			}
			t.Run(string(a)+"→"+string(b)+"→"+string(a), func(t *testing.T) {
				mid, err := Response(b, a, rtResponse(a))
				if err != nil {
					t.Fatal(err)
				}
				back, err := Response(a, b, mid)
				if err != nil {
					t.Fatal(err)
				}
				s := string(back)
				for _, want := range []string{"pong-p1", "hmm"} {
					if !strings.Contains(s, want) {
						t.Fatalf("lost %q on %s→%s→%s:\n%s", want, a, b, a, s)
					}
				}
				if a != config.ProtocolOpenAIChat && b != config.ProtocolOpenAIChat {
					if !strings.Contains(s, p2Sig) {
						t.Fatalf("lost Thinking Signature:\n%s", s)
					}
				}
				if a != config.ProtocolClaudeMessages && b != config.ProtocolClaudeMessages {
					if !strings.Contains(s, "17") {
						t.Fatalf("lost reasoning tokens:\n%s", s)
					}
				}
				if !strings.Contains(s, "13") {
					t.Fatalf("lost cache-read tokens:\n%s", s)
				}
				if a == config.ProtocolClaudeMessages && b != config.ProtocolClaudeMessages {
					if strings.Contains(s, `"cache_creation_input_tokens":11`) {
						t.Fatalf("cache-create must not return from a protocol with no analogue:\n%s", s)
					}
				}
			})
		}
	}
}
