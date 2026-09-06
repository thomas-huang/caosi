package convert

import (
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestRequest_GeminiPDF_NotImage(t *testing.T) {
	in := []byte(`{"contents":[{"role":"user","parts":[{"text":"read"},{"inlineData":{"mimeType":"application/pdf","data":"JVBERi0="}}]}]}`)
	chat, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if strings.Contains(s, `"type":"image_url"`) {
		t.Fatalf("PDF coerced to image:\n%s", s)
	}
	if !strings.Contains(s, `"type":"file"`) || !strings.Contains(s, "JVBERi0=") {
		t.Fatalf("document dropped:\n%s", s)
	}

	claude, err := Request(config.ProtocolGemini, config.ProtocolClaudeMessages, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	cs := string(claude)
	if strings.Contains(cs, `"type":"image"`) {
		t.Fatalf("PDF coerced to Claude image:\n%s", cs)
	}
	if !strings.Contains(cs, `"type":"document"`) || !strings.Contains(cs, "JVBERi0=") {
		t.Fatalf("Claude document dropped:\n%s", cs)
	}
}

func TestRequest_ChatHTTPImage_GeminiDropped_ClaudeKept(t *testing.T) {
	in := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"text","text":"see"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]}]}`)
	gemini, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, in, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	gs := string(gemini)
	if strings.Contains(gs, "example.com") || strings.Contains(gs, "inlineData") {
		t.Fatalf("HTTP image should drop toward Gemini:\n%s", gs)
	}
	if !strings.Contains(gs, "see") {
		t.Fatalf("text dropped:\n%s", gs)
	}

	claude, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, in, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	cs := string(claude)
	if !strings.Contains(cs, `"type":"url"`) || !strings.Contains(cs, "https://example.com/a.png") {
		t.Fatalf("HTTP image should keep toward Claude:\n%s", cs)
	}
}

func TestRequest_ClaudeDocument_ToChatAndGemini(t *testing.T) {
	in := []byte(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}},{"type":"text","text":"read"}]}]}`)
	chat, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if !strings.Contains(s, `"type":"file"`) || !strings.Contains(s, "JVBERi0=") {
		t.Fatalf("document→Chat file dropped:\n%s", s)
	}

	gemini, err := Request(config.ProtocolClaudeMessages, config.ProtocolGemini, in, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	gs := string(gemini)
	if !strings.Contains(gs, "inlineData") || !strings.Contains(gs, "application/pdf") {
		t.Fatalf("document→Gemini dropped:\n%s", gs)
	}
}

func TestRequest_DocumentURL_DroppedExceptClaude(t *testing.T) {
	in := []byte(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":[{"type":"document","source":{"type":"url","url":"https://example.com/a.pdf"}},{"type":"text","text":"read"}]}]}`)
	chat, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(chat), "example.com/a.pdf") {
		t.Fatalf("document URL must drop toward Chat:\n%s", chat)
	}

	gemini, err := Request(config.ProtocolClaudeMessages, config.ProtocolGemini, in, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gemini), "example.com/a.pdf") {
		t.Fatalf("document URL must drop toward Gemini:\n%s", gemini)
	}
}

func TestRequest_GeminiAudioVideo(t *testing.T) {
	in := []byte(`{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"audio/wav","data":"UklGRg=="}},{"inlineData":{"mimeType":"video/mp4","data":"AAAA"}},{"text":"what"}]}]}`)
	chat, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if strings.Contains(s, "video_url") {
		t.Fatalf("must not invent Chat video_url:\n%s", s)
	}
	if !strings.Contains(s, `"type":"input_audio"`) || !strings.Contains(s, "UklGRg==") {
		t.Fatalf("audio dropped toward Chat:\n%s", s)
	}

	claude, err := Request(config.ProtocolGemini, config.ProtocolClaudeMessages, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	cs := string(claude)
	if strings.Contains(cs, "UklGRg==") || strings.Contains(cs, "AAAA") {
		t.Fatalf("audio/video must drop toward Claude:\n%s", cs)
	}
	if !strings.Contains(cs, "what") {
		t.Fatalf("text dropped:\n%s", cs)
	}
}

func TestRequest_ClaudeToolResultNestedImage(t *testing.T) {
	in := []byte(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QQ=="}},{"type":"text","text":"ok"}]}]}]}`)
	chat, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if !strings.Contains(s, `"role":"tool"`) || !strings.Contains(s, "ok") {
		t.Fatalf("tool result text dropped:\n%s", s)
	}
	if strings.Contains(s, "image_url") || strings.Contains(s, "QQ==") {
		t.Fatalf("nested image must drop toward Chat tool content:\n%s", s)
	}

	claude, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, chat, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(claude), `"type":"image"`) {
		t.Fatalf("Chat tool string must not grow an image on the way back:\n%s", claude)
	}
}

func TestResponse_GeminiImage_OnlyGeminiClient(t *testing.T) {
	in := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"pong"},{"inlineData":{"mimeType":"image/png","data":"QQ=="}}]},"finishReason":"STOP"}]}`)
	chat, err := Response(config.ProtocolOpenAIChat, config.ProtocolGemini, in)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(chat), "QQ==") || strings.Contains(string(chat), "image_url") {
		t.Fatalf("assistant image must drop toward Chat:\n%s", chat)
	}
	if !strings.Contains(string(chat), "pong") {
		t.Fatalf("text dropped:\n%s", chat)
	}

	gemini, err := Response(config.ProtocolGemini, config.ProtocolOpenAIChat, []byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gemini), "pong") {
		t.Fatalf("gemini client text:\n%s", gemini)
	}

	back, err := Response(config.ProtocolGemini, config.ProtocolGemini, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(back), "QQ==") {
		t.Fatalf("passthrough should keep image:\n%s", back)
	}
}

func TestRequest_ResponsesFileData(t *testing.T) {
	in := []byte(`{"model":"gpt-x","input":[{"role":"user","content":[{"type":"input_file","filename":"a.pdf","file_data":"data:application/pdf;base64,JVBERi0="},{"type":"input_text","text":"read"}]}]}`)
	claude, err := Request(config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages, in, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(claude)
	if !strings.Contains(s, `"type":"document"`) || !strings.Contains(s, "JVBERi0=") {
		t.Fatalf("input_file→document dropped:\n%s", s)
	}
}
