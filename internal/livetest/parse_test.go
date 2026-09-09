package livetest

import (
	"bytes"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestParseClientResponse_JSON(t *testing.T) {
	cases := []struct {
		proto config.Protocol
		body  string
	}{
		{config.ProtocolOpenAIChat, `{"id":"1","choices":[{"message":{"content":"ok"}}]}`},
		{config.ProtocolOpenAIChat, `{"error":{"message":"nope"}}`},
		{config.ProtocolOpenAIResponses, `{"id":"r","object":"response","status":"completed","output":[]}`},
		{config.ProtocolClaudeMessages, `{"id":"m","type":"message","content":[{"type":"text","text":"ok"}]}`},
		{config.ProtocolGemini, `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`},
	}
	for _, c := range cases {
		if err := parseClientResponse(c.proto, false, []byte(c.body)); err != nil {
			t.Fatalf("%s: %v", c.proto, err)
		}
	}
	if err := parseClientResponse(config.ProtocolOpenAIChat, false, []byte(`{"ok":true}`)); err == nil {
		t.Fatal("want reject")
	}
	if err := parseClientResponse(config.ProtocolOpenAIChat, false, nil); err == nil {
		t.Fatal("empty")
	}
}

func TestParseClientResponse_Stream(t *testing.T) {
	chat := "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n"
	if err := parseClientResponse(config.ProtocolOpenAIChat, true, []byte(chat)); err != nil {
		t.Fatal(err)
	}
	claude := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\"}}\n\n"
	if err := parseClientResponse(config.ProtocolClaudeMessages, true, []byte(claude)); err != nil {
		t.Fatal(err)
	}
	gemini := `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}` + "\n"
	if err := parseClientResponse(config.ProtocolGemini, true, []byte(gemini)); err != nil {
		t.Fatal(err)
	}
	if err := parseClientResponse(config.ProtocolOpenAIChat, true, []byte("data: [DONE]\n")); err == nil {
		t.Fatal("DONE only")
	}
}

func TestRequestBody_Bundle(t *testing.T) {
	for _, client := range clientProtocols {
		for _, upstream := range clientProtocols {
			b, err := requestBody(client, false, bundleCells(client, upstream))
			if err != nil {
				t.Fatalf("%s→%s: %v", client, upstream, err)
			}
			if !bytes.Contains(b, []byte(systemText)) || !bytes.Contains(b, []byte("get_time")) || !bytes.Contains(b, []byte(userText)) {
				t.Fatalf("%s→%s missing system/tools/text: %s", client, upstream, b)
			}
			if applies(client, upstream, cellImage) && !bytes.Contains(b, []byte("image/png")) {
				t.Fatalf("%s→%s missing image: %s", client, upstream, b)
			}
			wantAudio := applies(client, upstream, cellAudio)
			hasAudio := bytes.Contains(b, []byte("input_audio")) || bytes.Contains(b, []byte("audio/wav"))
			if wantAudio != hasAudio {
				t.Fatalf("%s→%s audio want=%v got=%v", client, upstream, wantAudio, hasAudio)
			}
			wantVideo := applies(client, upstream, cellVideo)
			hasVideo := bytes.Contains(b, []byte("video/mp4"))
			if wantVideo != hasVideo {
				t.Fatalf("%s→%s video want=%v got=%v", client, upstream, wantVideo, hasVideo)
			}
			b2, err := requestBody(client, true, bundleCells(client, upstream))
			if err != nil {
				t.Fatalf("%s→%s stream: %v", client, upstream, err)
			}
			if client != config.ProtocolGemini && !bytes.Contains(b2, []byte(`"stream":true`)) {
				t.Fatalf("%s→%s stream flag missing", client, upstream)
			}
		}
	}
}

func TestRequestBody_SingleCellOmitsOthers(t *testing.T) {
	b, err := requestBody(config.ProtocolOpenAIChat, false, []cell{cellImage})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b, []byte("image/png")) || !bytes.Contains(b, []byte(userText)) {
		t.Fatalf("image cell: %s", b)
	}
	if bytes.Contains(b, []byte(systemText)) || bytes.Contains(b, []byte("get_time")) {
		t.Fatalf("image cell should not carry system/tools: %s", b)
	}
	b, err = requestBody(config.ProtocolClaudeMessages, false, []cell{cellText})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte("image/png")) || bytes.Contains(b, []byte("get_time")) || bytes.Contains(b, []byte(systemText)) {
		t.Fatalf("text cell extras: %s", b)
	}
}
