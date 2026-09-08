package convert

import (
	"bytes"
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

const p2Sig = "sig-p2-carriage"

func TestRequest_ThinkingSignatureCarriage(t *testing.T) {
	claude := []byte(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"hmm","signature":"sig-p2-carriage"},{"type":"text","text":"see-p1"}]}]}`)
	gemini := []byte(`{"contents":[{"role":"model","parts":[{"thought":true,"text":"hmm","thoughtSignature":"sig-p2-carriage"},{"text":"see-p1"}]}]}`)
	responses := []byte(`{"model":"gpt-x","input":[{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}],"encrypted_content":"sig-p2-carriage"},{"type":"message","role":"user","content":[{"type":"input_text","text":"see-p1"}]}]}`)

	assertSig := func(t *testing.T, src config.Protocol, body []byte) {
		t.Helper()
		for _, dst := range allProtocols {
			if src == dst {
				continue
			}
			t.Run(string(src)+"→"+string(dst), func(t *testing.T) {
				out, err := Request(src, dst, body, "m", false)
				if err != nil {
					t.Fatal(err)
				}
				s := string(out)
				if !strings.Contains(s, "hmm") {
					t.Fatalf("thinking text dropped:\n%s", s)
				}
				switch dst {
				case config.ProtocolClaudeMessages:
					mustHave(t, s, `"signature":"sig-p2-carriage"`)
				case config.ProtocolGemini:
					mustHave(t, s, `"thoughtSignature":"sig-p2-carriage"`)
				case config.ProtocolOpenAIResponses:
					mustHave(t, s, `"encrypted_content":"sig-p2-carriage"`)
				case config.ProtocolOpenAIChat:
					mustOmit(t, s, p2Sig, "thoughtSignature", "encrypted_content", `"signature"`)
					if !strings.Contains(s, "hmm") {
						t.Fatalf("thinking text dropped toward Chat:\n%s", s)
					}
				}
			})
		}
	}
	t.Run("from claude", func(t *testing.T) { assertSig(t, config.ProtocolClaudeMessages, claude) })
	t.Run("from gemini", func(t *testing.T) { assertSig(t, config.ProtocolGemini, gemini) })
	t.Run("from responses", func(t *testing.T) { assertSig(t, config.ProtocolOpenAIResponses, responses) })
}

func TestRequest_ThinkingSignature_ChatDrops(t *testing.T) {
	chat := []byte(`{"model":"gpt-x","messages":[{"role":"assistant","content":"see-p1","reasoning_content":"hmm"}]}`)
	for _, dst := range []config.Protocol{config.ProtocolClaudeMessages, config.ProtocolGemini, config.ProtocolOpenAIResponses} {
		out, err := Request(config.ProtocolOpenAIChat, dst, chat, "m", false)
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		t.Run("chat→"+string(dst), func(t *testing.T) {
			if !strings.Contains(s, "hmm") {
				t.Fatalf("thinking text dropped:\n%s", s)
			}
			mustOmit(t, s, p2Sig, "thoughtSignature", "encrypted_content", `"signature"`)
		})
	}
}

func TestRequest_GeminiFunctionCallSignature_Dropped(t *testing.T) {
	in := []byte(`{"contents":[{"role":"model","parts":[{"functionCall":{"name":"Glob","args":{"pattern":"*.go"}},"thoughtSignature":"sig-p2-carriage"},{"text":"see-p1"}]}]}`)
	for _, dst := range []config.Protocol{config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses} {
		out, err := Request(config.ProtocolGemini, dst, in, "m", false)
		if err != nil {
			t.Fatal(err)
		}
		s := string(out)
		t.Run("fn→"+string(dst), func(t *testing.T) {
			mustOmit(t, s, p2Sig, "thoughtSignature", `"signature":"sig-p2-carriage"`, "encrypted_content")
			if !strings.Contains(s, "Glob") {
				t.Fatalf("tool call dropped:\n%s", s)
			}
			if strings.Contains(s, `"type":"thinking"`) {
				t.Fatalf("must not invent thinking to carry functionCall signature:\n%s", s)
			}
		})
	}
}

func TestResponse_ThinkingSignatureCarriage(t *testing.T) {
	claude := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"thinking","thinking":"hmm","signature":"sig-p2-carriage"},{"type":"text","text":"pong-p1"}],"stop_reason":"end_turn"}`)
	gemini := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"hmm","thoughtSignature":"sig-p2-carriage"},{"text":"pong-p1"}]},"finishReason":"STOP"}]}`)
	responses := []byte(`{"id":"resp_1","object":"response","status":"completed","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}],"encrypted_content":"sig-p2-carriage"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong-p1"}]}]}`)

	check := func(t *testing.T, upstream config.Protocol, body []byte) {
		t.Helper()
		for _, client := range allProtocols {
			if client == upstream {
				continue
			}
			t.Run(string(upstream)+"→"+string(client), func(t *testing.T) {
				s := p1Resp(t, client, upstream, body)
				switch client {
				case config.ProtocolClaudeMessages:
					mustHave(t, s, `"signature":"sig-p2-carriage"`)
				case config.ProtocolGemini:
					mustHave(t, s, `"thoughtSignature":"sig-p2-carriage"`)
				case config.ProtocolOpenAIResponses:
					mustHave(t, s, `"encrypted_content":"sig-p2-carriage"`)
				case config.ProtocolOpenAIChat:
					mustOmit(t, s, p2Sig, "thoughtSignature", "encrypted_content")
				}
			})
		}
	}
	t.Run("claude up", func(t *testing.T) { check(t, config.ProtocolClaudeMessages, claude) })
	t.Run("gemini up", func(t *testing.T) { check(t, config.ProtocolGemini, gemini) })
	t.Run("responses up", func(t *testing.T) { check(t, config.ProtocolOpenAIResponses, responses) })
}

func TestStream_ClaudeFromResponses_CarriesSignature(t *testing.T) {
	in := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}`,
		``,
		`event: response.reasoning_summary_text.delta`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"hmm"}`,
		``,
		`event: response.reasoning.encrypted_content`,
		`data: {"type":"response.reasoning.encrypted_content","encrypted_content":"sig-p2-carriage"}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"ok"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	mustHave(t, s, `"type":"thinking_delta"`, "hmm", `"type":"signature_delta"`, p2Sig, "ok")
}

func TestStream_ClaudeFromResponsesItem_CarriesSignature(t *testing.T) {
	in := strings.Join([]string{
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","item":{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}],"encrypted_content":"sig-p2-carriage"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed"}}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	mustHave(t, s, `"type":"signature_delta"`, p2Sig, "hmm")
}

func TestStream_IncrementalThinking_NoSignatureFromChat(t *testing.T) {
	in := strings.Join([]string{
		`data: {"id":"chatcmpl-1","choices":[{"delta":{"reasoning_content":"hmm"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"ok"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")

	t.Run("claude←chat", func(t *testing.T) {
		var out bytes.Buffer
		if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
			t.Fatal(err)
		}
		s := out.String()
		mustHave(t, s, `"type":"thinking_delta"`, "hmm")
		mustOmit(t, s, "signature_delta", p2Sig)
	})
	t.Run("responses←chat", func(t *testing.T) {
		var out bytes.Buffer
		if err := Stream(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
			t.Fatal(err)
		}
		s := out.String()
		mustOmit(t, s, "encrypted_content", p2Sig)
	})
	t.Run("gemini←chat", func(t *testing.T) {
		var out bytes.Buffer
		if err := Stream(config.ProtocolGemini, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
			t.Fatal(err)
		}
		s := out.String()
		mustOmit(t, s, "thoughtSignature", p2Sig)
	})
}

func TestStream_ClaudeFromGeminiJSON_CarriesThoughtSignature(t *testing.T) {
	in := `{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"hmm","thoughtSignature":"sig-p2-carriage"},{"text":"ok"}]},"finishReason":"STOP"}]}`
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolGemini, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	mustHave(t, s, `"type":"thinking_delta"`, "hmm", `"type":"signature_delta"`, p2Sig)
}
