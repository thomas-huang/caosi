package convert

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

func TestRequest_ChatVideoHTTPURL_Dropped(t *testing.T) {
	in := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"text","text":"watch"},{"type":"video_url","video_url":{"url":"https://example.com/clip.mp4"}}]}]}`)
	gemini, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, in, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	gs := string(gemini)
	if !strings.Contains(gs, "watch") {
		t.Fatalf("text dropped:\n%s", gs)
	}
	if strings.Contains(gs, "example.com") || strings.Contains(gs, "inlineData") {
		t.Fatalf("HTTP video must drop toward Gemini (no fetch):\n%s", gs)
	}

	claude, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, in, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(claude), "example.com/clip.mp4") {
		t.Fatalf("HTTP video must drop toward Claude:\n%s", claude)
	}
	if !strings.Contains(string(claude), "watch") {
		t.Fatalf("text dropped:\n%s", claude)
	}

	http := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"video_url","video_url":"http://cdn.example/a.mp4"},{"type":"text","text":"ok"}]}]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, http, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "cdn.example") {
		t.Fatalf("http:// video must drop:\n%s", out)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("text dropped:\n%s", out)
	}
}

func TestRequest_ChatVideoDataURL_GeminiKeeps(t *testing.T) {
	in := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"video_url","video_url":"data:video/mp4;base64,AAAA"},{"type":"text","text":"see"}]}]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, in, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "AAAA") || !strings.Contains(s, "video/mp4") {
		t.Fatalf("inline video dropped:\n%s", s)
	}
	if !strings.Contains(s, "see") {
		t.Fatalf("text dropped:\n%s", s)
	}

	bare := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"video_url","video_url":{"url":"data:;base64,BBBB"}}]}]}`)
	out, err = Request(config.ProtocolOpenAIChat, config.ProtocolGemini, bare, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "BBBB") {
		t.Fatalf("data URL without mime dropped:\n%s", out)
	}

	dropped := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"video_url","video_url":{"url":"ftp://x/a.mp4"}},{"type":"text","text":"keep"}]}]}`)
	out, err = Request(config.ProtocolOpenAIChat, config.ProtocolGemini, dropped, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "ftp://") {
		t.Fatalf("non-http video must drop:\n%s", out)
	}
	if !strings.Contains(string(out), "keep") {
		t.Fatalf("text dropped:\n%s", out)
	}
}

func TestRequest_ChatInputAudio_Formats(t *testing.T) {
	wav := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"UklGRg==","format":"wav"}},{"type":"text","text":"hear"}]}]}`)
	gemini, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, wav, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	gs := string(gemini)
	if !strings.Contains(gs, "UklGRg==") || !strings.Contains(gs, "audio/wav") {
		t.Fatalf("wav audio dropped or wrong mime:\n%s", gs)
	}
	if !strings.Contains(gs, "hear") {
		t.Fatalf("text dropped:\n%s", gs)
	}

	mp3 := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"//uQ","format":"MP3"}}]}]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, mp3, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "audio/mpeg") || !strings.Contains(string(out), "//uQ") {
		t.Fatalf("mp3 mime:\n%s", out)
	}

	unknown := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"QQ==","format":"flac"}}]}]}`)
	out, err = Request(config.ProtocolOpenAIChat, config.ProtocolGemini, unknown, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "QQ==") || !strings.Contains(string(out), "audio/mpeg") {
		t.Fatalf("unknown format should default to audio/mpeg:\n%s", out)
	}

	chat, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, wav, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chat), `"type":"input_audio"`) || !strings.Contains(string(chat), "UklGRg==") {
		t.Fatalf("passthrough should keep wav:\n%s", chat)
	}

	empty := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"format":"wav"}},{"type":"text","text":"x"}]}]}`)
	out, err = Request(config.ProtocolOpenAIChat, config.ProtocolGemini, empty, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "inlineData") {
		t.Fatalf("empty audio data must drop:\n%s", out)
	}
}

func TestRequest_ChatTool_ContentAsString(t *testing.T) {
	nilContent := []byte(`{"model":"gpt-x","messages":[{"role":"tool","tool_call_id":"call_1"}]}`)
	claude, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, nilContent, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	cs := string(claude)
	if !strings.Contains(cs, `"type":"tool_result"`) || !strings.Contains(cs, "call_1") {
		t.Fatalf("nil tool content dropped tool_result:\n%s", cs)
	}

	empty := []byte(`{"model":"gpt-x","messages":[{"role":"tool","tool_call_id":"call_2","content":""}]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, empty, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "call_2") {
		t.Fatalf("empty tool content:\n%s", out)
	}

	obj := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":{"note":"plain-object"}}]}`)
	chat, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, obj, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chat), "plain-object") {
		t.Fatalf("object content should stringify:\n%s", chat)
	}

	num := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":42}]}`)
	out, err = Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, num, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "42") {
		t.Fatalf("numeric content:\n%s", out)
	}
}

func TestHelpers_HTTPURL_AudioFormat_ContentAsString(t *testing.T) {
	if !isHTTPURL("https://example.com/a.png") {
		t.Fatal("https should be accepted")
	}
	if !isHTTPURL("http://localhost/x") {
		t.Fatal("http should be accepted")
	}
	if isHTTPURL("data:image/png;base64,QQ==") || isHTTPURL("ftp://x") || isHTTPURL("") {
		t.Fatal("non-http URLs must be rejected")
	}

	if got := mimeFromAudioFormat("wav"); got != "audio/wav" {
		t.Fatalf("wav mime=%q", got)
	}
	if got := mimeFromAudioFormat(" MP3 "); got != "audio/mpeg" {
		t.Fatalf("mp3 mime=%q", got)
	}
	if got := mimeFromAudioFormat("ogg"); got != "" {
		t.Fatalf("unknown format should be empty, got %q", got)
	}

	if got := contentAsString("hello"); got != "hello" {
		t.Fatalf("string=%q", got)
	}
	if got := contentAsString(nil); got != "" {
		t.Fatalf("nil=%q", got)
	}
	parts := []openaiPart{{Type: "text", Text: "ab"}, {Type: "text", Text: "cd"}}
	if got := contentAsString(parts); got != "abcd" {
		t.Fatalf("parts=%q", got)
	}
	if got := contentAsString(map[string]int{"n": 1}); !strings.Contains(got, `"n":1`) && !strings.Contains(got, `"n": 1`) {
		t.Fatalf("object stringify=%q", got)
	}
}

func TestRequest_ChatMapToIRPart_Kinds(t *testing.T) {
	in := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[
		{"type":"text","text":"see"},
		{"type":"image_url","image_url":"data:image/jpeg;base64,QQ=="},
		{"type":"image","image_url":{"url":"data:image/png;base64,ww=="}},
		{"type":"file","file":{"filename":"a.pdf","file_data":"data:application/pdf;base64,JVBERi0="}},
		{"type":"file","file":{"file_id":"file-1"}},
		{"type":"file","file":{"file_data":"not-a-data-url","filename":"raw.bin"}},
		{"text":"bare"},
		{"type":"unknown","text":"nope"}
	]}]}`)
	claude, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, in, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(claude)
	if !strings.Contains(s, "see") || !strings.Contains(s, "bare") {
		t.Fatalf("text parts dropped:\n%s", s)
	}
	if !strings.Contains(s, "QQ==") || !strings.Contains(s, "ww==") {
		t.Fatalf("images dropped:\n%s", s)
	}
	if !strings.Contains(s, `"type":"document"`) || !strings.Contains(s, "JVBERi0=") {
		t.Fatalf("file_data document dropped:\n%s", s)
	}
	if strings.Contains(s, "file-1") {
		t.Fatalf("file_id-only must drop:\n%s", s)
	}
	if strings.Contains(s, "nope") {
		t.Fatalf("unknown type must drop:\n%s", s)
	}

	raw, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	rs := string(raw)
	if !strings.Contains(rs, `"type":"file"`) || !strings.Contains(rs, "raw.bin") {
		t.Fatalf("raw file_data should become Chat file:\n%s", rs)
	}

	emptyImg := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":""}},{"type":"text","text":"x"}]}]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, emptyImg, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"type":"image"`) {
		t.Fatalf("empty image url must drop:\n%s", out)
	}
}

func TestRequest_ClaudeToolChoice(t *testing.T) {
	base := `{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"get_weather","description":"w","input_schema":{"type":"object"}}],"tool_choice":%s}`
	cases := []struct {
		choice string
		want   string
	}{
		{`"auto"`, `"auto"`},
		{`"none"`, `"none"`},
		{`"any"`, `"required"`},
		{`"required"`, `"required"`},
		{`{"type":"tool","name":"get_weather"}`, `"type":"function"`},
		{`null`, ""},
		{`"bogus"`, ""},
	}
	for _, tc := range cases {
		in := []byte(strings.Replace(base, "%s", tc.choice, 1))
		out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", false)
		if err != nil {
			t.Fatalf("choice %s: %v", tc.choice, err)
		}
		s := string(out)
		if !strings.Contains(s, "hi") {
			t.Fatalf("text dropped for %s:\n%s", tc.choice, s)
		}
		if tc.want == "" {
			if strings.Contains(s, `"tool_choice"`) && (tc.choice == `null` || tc.choice == `"bogus"`) {
				// omitempty may drop it; if present it must not be the bogus string
				if strings.Contains(s, `"bogus"`) {
					t.Fatalf("bogus tool_choice leaked:\n%s", s)
				}
			}
			continue
		}
		if !strings.Contains(s, tc.want) {
			t.Fatalf("tool_choice %s missing %q:\n%s", tc.choice, tc.want, s)
		}
		if strings.Contains(tc.choice, "get_weather") && !strings.Contains(s, "get_weather") {
			t.Fatalf("named tool missing:\n%s", s)
		}
	}
}

func TestRequest_ClaudeSystemBlocksAndThinking(t *testing.T) {
	in := []byte(`{
	  "model":"claude-opus","max_tokens":8,
	  "system":[{"type":"text","text":"sys-a"},{"type":"text","text":"sys-b"}],
	  "thinking":{"type":"enabled","budget_tokens":16000},
	  "messages":[{"role":"user","content":"hi"}],
	  "stop_sequences":["END"]
	}`)
	chat, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", true)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if !strings.Contains(s, "sys-asys-b") && !(strings.Contains(s, "sys-a") && strings.Contains(s, "sys-b")) {
		t.Fatalf("system blocks:\n%s", s)
	}
	if !strings.Contains(s, `"reasoning_effort":"high"`) {
		t.Fatalf("high budget thinking:\n%s", s)
	}
	if !strings.Contains(s, `"stream":true`) {
		t.Fatalf("stream flag:\n%s", s)
	}

	low := []byte(`{"model":"c","max_tokens":8,"thinking":{"type":"enabled","budget_tokens":100},"messages":[{"role":"user","content":"hi"}]}`)
	out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, low, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"reasoning_effort":"low"`) {
		t.Fatalf("low budget:\n%s", out)
	}

	disabled := []byte(`{"model":"c","max_tokens":8,"thinking":{"type":"disabled"},"messages":[{"role":"user","content":"hi"}]}`)
	out, err = Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, disabled, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"reasoning_effort":"low"`) {
		t.Fatalf("disabled thinking:\n%s", out)
	}

	emptyRole := []byte(`{"model":"c","max_tokens":8,"messages":[{"content":"anon"}]}`)
	out, err = Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, emptyRole, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"role":"user"`) || !strings.Contains(string(out), "anon") {
		t.Fatalf("empty role:\n%s", out)
	}

	thinkText := []byte(`{"model":"c","max_tokens":8,"messages":[{"role":"assistant","content":[{"type":"thinking","text":"secret"},{"type":"text","text":"hi"}]}]}`)
	out, err = Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, thinkText, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "secret") {
		t.Fatalf("thinking text field:\n%s", out)
	}
}

func TestRequest_GeminiAltsAndFunctionResponse(t *testing.T) {
	in := []byte(`{
	  "generationConfig":{"thinkingConfig":{},"temperature":0.2,"topP":0.9,"maxOutputTokens":16},
	  "contents":[
	    {"role":"user","parts":[{"inline_data":{"mimeType":"image/jpeg","data":"qq=="}},{"text":"see"}]},
	    {"role":"model","parts":[{"function_call":{"name":"lookup","args":{"q":"x"}}}]},
	    {"role":"user","parts":[{"functionResponse":{"name":"lookup","response":{"ok":true}}}]}
	  ]
	}`)
	chat, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, in, "", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if !strings.Contains(s, "qq==") || !strings.Contains(s, "see") {
		t.Fatalf("inline_data/text:\n%s", s)
	}
	if !strings.Contains(s, "lookup") {
		t.Fatalf("function_call:\n%s", s)
	}
	if !strings.Contains(s, `"role":"tool"`) {
		t.Fatalf("functionResponse:\n%s", s)
	}
	if !strings.Contains(s, `"reasoning_effort":"medium"`) {
		t.Fatalf("empty thinkingLevel defaults medium:\n%s", s)
	}

	empty := []byte(`{"contents":[{"role":"user","parts":[{}]}]}`)
	out, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, empty, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"role":"user"`) {
		t.Fatalf("empty parts:\n%s", out)
	}
}

func TestRequest_ResponsesInputVariants(t *testing.T) {
	strIn := []byte(`{"model":"gpt-x","input":"just text"}`)
	out, err := Request(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, strIn, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "just text") {
		t.Fatalf("string input:\n%s", out)
	}

	tools := []byte(`{"model":"gpt-x","input":[
		{"type":"message","role":"system","content":[{"type":"input_text","text":"sys"}]},
		{"type":"function_call","call_id":"c1","name":"fn","arguments":""},
		{"type":"function_call_output","call_id":"c1","output":"done"},
		{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm"}]},
		{"role":"user","content":[{"type":"input_image","image_url":{"url":"data:image/png;base64,QQ=="}},{"type":"input_file","file":{"file_data":"data:text/plain;base64,aGk="},"filename":"a.txt"}]}
	],"tools":[{"type":"function","function":{"name":"fn","description":"d","parameters":{"type":"object"}}},{"type":"function","name":""}]}`)
	out, err = Request(config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages, tools, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "sys") || !strings.Contains(s, "fn") || !strings.Contains(s, "done") {
		t.Fatalf("responses items:\n%s", s)
	}
	if !strings.Contains(s, "QQ==") {
		t.Fatalf("nested image_url object:\n%s", s)
	}

	nullIn := []byte(`{"model":"gpt-x","input":null}`)
	out, err = Request(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, nullIn, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"role":"user"`) && strings.Contains(string(out), `"content":null`) {
		t.Fatalf("null input should not invent user text:\n%s", out)
	}
}

func TestRequest_DocumentFilenamesFromMIME(t *testing.T) {
	cases := []struct {
		mime string
		want string
	}{
		{"application/pdf", "document.pdf"},
		{"text/plain", "document.txt"},
		{"text/csv", "document.csv"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "document.docx"},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "document.xlsx"},
		{"application/zip", "document.bin"},
	}
	for _, tc := range cases {
		in := []byte(`{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"` + tc.mime + `","data":"QQ=="}},{"text":"read"}]}]}`)
		out, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, in, "m", false)
		if err != nil {
			t.Fatalf("%s: %v", tc.mime, err)
		}
		s := string(out)
		if !strings.Contains(s, `"type":"file"`) || !strings.Contains(s, tc.want) {
			t.Fatalf("%s filename %s missing:\n%s", tc.mime, tc.want, s)
		}
		resp, err := Request(config.ProtocolGemini, config.ProtocolOpenAIResponses, in, "m", false)
		if err != nil {
			t.Fatalf("responses %s: %v", tc.mime, err)
		}
		if !strings.Contains(string(resp), tc.want) {
			t.Fatalf("responses filename %s:\n%s", tc.want, resp)
		}
	}
}

func TestRequest_GeminiAudioMIMEToChat(t *testing.T) {
	for _, mime := range []string{"audio/wav", "audio/wave", "audio/mpeg", "audio/mp3"} {
		in := []byte(`{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"` + mime + `","data":"UklGRg=="}},{"text":"hear"}]}]}`)
		out, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, in, "m", false)
		if err != nil {
			t.Fatalf("%s: %v", mime, err)
		}
		if !strings.Contains(string(out), `"type":"input_audio"`) || !strings.Contains(string(out), "UklGRg==") {
			t.Fatalf("%s audio dropped:\n%s", mime, out)
		}
	}
	other := []byte(`{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"audio/ogg","data":"QQ=="}},{"text":"hear"}]}]}`)
	out, err := Request(config.ProtocolGemini, config.ProtocolOpenAIChat, other, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"type":"input_audio"`) {
		t.Fatalf("ogg is not a Chat analogue:\n%s", out)
	}
}

func TestRequest_UnknownProtocolAndInvalidJSON(t *testing.T) {
	_, err := Request(config.Protocol("nope"), config.ProtocolOpenAIChat, []byte(`{"messages":[]}`), "m", false)
	if err == nil || !strings.Contains(err.Error(), "未知") {
		t.Fatalf("want unknown client, got %v", err)
	}
	_, err = Request(config.ProtocolOpenAIChat, config.Protocol("nope"), []byte(`{"messages":[{"role":"user","content":"hi"}]}`), "m", false)
	if err == nil || !strings.Contains(err.Error(), "未知") {
		t.Fatalf("want unknown upstream, got %v", err)
	}
	_, err = Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, []byte(`{`), "m", false)
	if err == nil {
		t.Fatal("invalid chat JSON")
	}
	_, err = Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, []byte(`{`), "m", false)
	if err == nil {
		t.Fatal("invalid claude JSON")
	}
	_, err = Request(config.ProtocolGemini, config.ProtocolOpenAIChat, []byte(`{`), "m", false)
	if err == nil {
		t.Fatal("invalid gemini JSON")
	}
	_, err = Request(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, []byte(`{`), "m", false)
	if err == nil {
		t.Fatal("invalid responses JSON")
	}
}

func TestApplyModel_EmptyInvalidAndBlank(t *testing.T) {
	in := []byte(`{"model":"old","messages":[]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, in, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, in) {
		t.Fatalf("empty model should leave body, got %s", out)
	}
	empty, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, nil, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty body: %q", empty)
	}
	bad := []byte(`not-json`)
	got, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, bad, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, bad) {
		t.Fatalf("invalid json passthrough: %s", got)
	}
}

func TestResponse_ChatVariantsAndFinishReasons(t *testing.T) {
	empty := []byte(`{"id":"x","model":"m","choices":[]}`)
	out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, empty)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"message"`) {
		t.Fatalf("empty choices:\n%s", out)
	}

	audio := []byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"hi","audio":{"data":"UklGRg=="}},"finish_reason":"stop"}]}`)
	chat, err := Response(config.ProtocolOpenAIChat, config.ProtocolGemini, []byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chat), "hi") {
		t.Fatalf("gemini→chat:\n%s", chat)
	}
	back, err := Response(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, audio)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(back), "UklGRg==") {
		t.Fatalf("passthrough audio:\n%s", back)
	}

	tools := []byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"fn","arguments":""}}]},"finish_reason":"function_call"}]}`)
	claude, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, tools)
	if err != nil {
		t.Fatal(err)
	}
	cs := string(claude)
	if !strings.Contains(cs, `"type":"tool_use"`) || !strings.Contains(cs, "fn") {
		t.Fatalf("tool_calls:\n%s", cs)
	}
	if !strings.Contains(cs, "tool_use") {
		t.Fatalf("stop_reason:\n%s", cs)
	}

	length := []byte(`{"choices":[{"message":{"content":"x"},"finish_reason":"length"}]}`)
	out, err = Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, length)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "max_tokens") {
		t.Fatalf("length→max_tokens:\n%s", out)
	}

	filter := []byte(`{"choices":[{"message":{"content":"x"},"finish_reason":"content_filter"}]}`)
	out, err = Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, filter)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "refusal") {
		t.Fatalf("content_filter→refusal:\n%s", out)
	}

	noID := []byte(`{"candidates":[{"content":{"parts":[{"text":"pong"}]},"finishReason":"MAX_TOKENS"}]}`)
	out, err = Response(config.ProtocolOpenAIChat, config.ProtocolGemini, noID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "pong") || !strings.Contains(string(out), `"finish_reason":"length"`) {
		t.Fatalf("gemini max tokens:\n%s", out)
	}

	_, err = Response(config.ProtocolOpenAIChat, config.Protocol("nope"), []byte(`{"id":"x"}`))
	if err == nil {
		t.Fatal("unknown upstream response")
	}
	_, err = Response(config.Protocol("nope"), config.ProtocolOpenAIChat, []byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}]}`))
	if err == nil {
		t.Fatal("unknown client response")
	}
	_, err = Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, []byte(`{`))
	if err == nil {
		t.Fatal("invalid chat response JSON")
	}
}

func TestResponse_ClaudeAndResponsesTools(t *testing.T) {
	claude := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"tool_use","id":"t1","name":"fn","input":{"a":1}},{"type":"thinking","thinking":""},{"type":"text","text":""}],"stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":2}}`)
	chat, err := Response(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, claude)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chat), `"tool_calls"`) || !strings.Contains(string(chat), "fn") {
		t.Fatalf("claude tools→chat:\n%s", chat)
	}

	emptyArgs := []byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"tool_use","id":"t1","name":"fn"}],"stop_reason":"max_tokens"}`)
	out, err := Response(config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages, emptyArgs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"function_call"`) {
		t.Fatalf("empty args:\n%s", out)
	}

	resp := []byte(`{"id":"resp_1","object":"response","status":"completed","output":[{"type":"function_call","call_id":"c1","name":"fn","arguments":""},{"type":"reasoning","summary":[]}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	out, err = Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, resp)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"tool_use"`) {
		t.Fatalf("responses function_call:\n%s", out)
	}

	errBody := []byte(`{"error":{"message":"nope","type":"server_error"}}`)
	out, err = Response(config.ProtocolOpenAIResponses, config.ProtocolGemini, errBody)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "nope") {
		t.Fatalf("gemini error→responses:\n%s", out)
	}

	gErr := []byte(`{"error":{"message":"boom","status":"INTERNAL"}}`)
	out, err = Response(config.ProtocolOpenAIChat, config.ProtocolGemini, gErr)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "boom") {
		t.Fatalf("gemini error body:\n%s", out)
	}

	emptyCand := []byte(`{"candidates":[]}`)
	out, err = Response(config.ProtocolOpenAIChat, config.ProtocolGemini, emptyCand)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"choices"`) {
		t.Fatalf("empty candidates:\n%s", out)
	}
}

func TestClientError_Shapes(t *testing.T) {
	st, body, ct := ClientError(config.ProtocolGemini, 200, "bad")
	if st != 400 {
		t.Fatalf("gemini status %d", st)
	}
	if ct != "application/json" || !strings.Contains(string(body), "INVALID_ARGUMENT") {
		t.Fatalf("gemini 4xx: %s", body)
	}
	_, body, _ = ClientError(config.ProtocolGemini, 503, "down")
	if !strings.Contains(string(body), "INTERNAL") {
		t.Fatalf("gemini 5xx: %s", body)
	}
	_, body, _ = ClientError(config.ProtocolClaudeMessages, 400, "x")
	if !strings.Contains(string(body), `"type":"error"`) {
		t.Fatalf("claude: %s", body)
	}
	_, body, _ = ClientError(config.ProtocolOpenAIChat, 400, "x")
	if !strings.Contains(string(body), "invalid_request_error") {
		t.Fatalf("openai: %s", body)
	}
}

func TestLooksLikeError_NonObject(t *testing.T) {
	out, err := Response(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, []byte(`{"error":"plain"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"type":"error"`) {
		t.Fatalf("string error field:\n%s", out)
	}
	out, err = Response(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, []byte(`not-json`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "not-json" {
		t.Fatalf("passthrough non-json: %s", out)
	}
}

func TestStream_ChatToClaude_ThinkingToolsErrorAndGarbage(t *testing.T) {
	in := strings.Join([]string{
		`not-json`,
		`data: {"id":"chatcmpl-1","model":"m","choices":[{"delta":{"reasoning_content":"hmm"}}]}`,
		``,
		`data: {"choices":[{"delta":{"content":"hi"}}]}`,
		``,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"fn","arguments":"{\"a\":1}"}}]}}]}`,
		``,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{`"type":"thinking_delta"`, "hmm", `"type":"text_delta"`, "hi", `"type":"tool_use"`, "fn", `"type":"input_json_delta"`, "tool_use", "event: message_stop"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}

	errIn := `data: {"error":{"message":"quota","type":"insufficient_quota"}}` + "\n\n"
	out.Reset()
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(errIn), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "event: error") || !strings.Contains(out.String(), "quota") {
		t.Fatalf("chat stream error:\n%s", out.String())
	}
}

type flushBuf struct {
	bytes.Buffer
	flushed int
}

func (f *flushBuf) Flush() { f.flushed++ }

func TestStream_WriteSSEFlushes(t *testing.T) {
	in := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"x"},"finish_reason":"stop"}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out flushBuf
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if out.flushed == 0 {
		t.Fatal("writeSSE should Flush when writer supports it")
	}
	if !strings.Contains(out.String(), "x") {
		t.Fatalf("lost text:\n%s", out.String())
	}

	out.Reset()
	out.flushed = 0
	if err := Stream(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if out.flushed == 0 {
		t.Fatal("responses event should Flush")
	}
}

func TestStream_AssembleClaudeAndResponsesEnvelopes(t *testing.T) {
	claude := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[]}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"thinking":"hmm"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","delta":{"text":"hi"}}`,
		``,
		`garbage`,
		`data: [DONE]`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, strings.NewReader(claude), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "hi") || !strings.Contains(s, "hmm") {
		t.Fatalf("assembled claude deltas:\n%s", s)
	}

	completed := strings.Join([]string{
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_1","object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}]}}`,
		``,
	}, "\n")
	out.Reset()
	if err := Stream(config.ProtocolGemini, config.ProtocolOpenAIResponses, strings.NewReader(completed), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "pong") || !strings.Contains(out.String(), `"candidates"`) {
		t.Fatalf("responses envelope→gemini:\n%s", out.String())
	}

	msg := `{"type":"message","id":"msg_1","role":"assistant","content":[{"type":"text","text":"yo"}]}`
	out.Reset()
	if err := Stream(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, strings.NewReader(msg), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "yo") {
		t.Fatalf("complete claude json:\n%s", out.String())
	}

	wrapped := `{"type":"response.incomplete","response":null}`
	out.Reset()
	if err := Stream(config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, strings.NewReader(wrapped), &out); err != nil {
		t.Fatal(err)
	}
}

func TestStream_ResponsesToClaude_FailedIncompleteCustomTool(t *testing.T) {
	in := strings.Join([]string{
		`event: response.failed`,
		`data: {"type":"response.failed","error":{"message":"boom","type":"server_error"}}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "event: error") || !strings.Contains(out.String(), "boom") {
		t.Fatalf("failed:\n%s", out.String())
	}

	inc := strings.Join([]string{
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","text":"hel"}`,
		``,
		`event: response.incomplete`,
		`data: {"type":"response.incomplete","response":{"id":"r1","status":"incomplete","usage":{"input_tokens":1,"output_tokens":2}}}`,
		``,
	}, "\n")
	out.Reset()
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(inc), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "hel") || !strings.Contains(s, "max_tokens") {
		t.Fatalf("incomplete:\n%s", s)
	}

	custom := strings.Join([]string{
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","item":{"id":"ct_1","type":"custom_tool_call","name":"do","call_id":"call_9","arguments":"{\"x\":1}"}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"r1","status":"completed"}}`,
		``,
	}, "\n")
	out.Reset()
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(custom), &out); err != nil {
		t.Fatal(err)
	}
	s = out.String()
	if !strings.Contains(s, "do") || !strings.Contains(s, `"type":"tool_use"`) {
		t.Fatalf("custom_tool_call:\n%s", s)
	}

	done := "data: [DONE]\n\n"
	out.Reset()
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(done), &out); err != nil {
		t.Fatal(err)
	}
}

func TestStream_ChatToGemini_GarbageAndFlush(t *testing.T) {
	in := strings.Join([]string{
		`data: nope`,
		``,
		`data: {"choices":[{"delta":{"content":"ok"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	var out flushBuf
	if err := Stream(config.ProtocolGemini, config.ProtocolOpenAIChat, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ok") || !strings.Contains(out.String(), `"candidates"`) {
		t.Fatalf("gemini stream:\n%s", out.String())
	}
}

func TestStream_ReadError(t *testing.T) {
	err := Stream(config.ProtocolOpenAIChat, config.ProtocolGemini, errReader{}, io.Discard)
	if err == nil {
		t.Fatal("want read error")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestRequest_IRRoundTripToolsEmptyParamsAndTemp(t *testing.T) {
	temp := 0.5
	_ = temp
	in := []byte(`{"model":"gpt-x","temperature":0.5,"top_p":0.9,"max_tokens":8,"stop":["END"],"messages":[{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"fn"}}]},{"role":"tool","tool_call_id":"c1","content":[{"type":"text","text":"ok"}]}],"tools":[{"type":"function","function":{"name":"fn"}}]}`)
	claude, err := Request(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, in, "c", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(claude)
	if !strings.Contains(s, "fn") || !strings.Contains(s, "ok") {
		t.Fatalf("tools round trip:\n%s", s)
	}
	if !strings.Contains(s, `"temperature":0.5`) && !strings.Contains(s, `"temperature": 0.5`) {
		t.Fatalf("temperature:\n%s", s)
	}

	gemini, err := Request(config.ProtocolOpenAIChat, config.ProtocolGemini, in, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gemini), "functionResponse") && !strings.Contains(string(gemini), "function_response") {
		t.Fatalf("gemini tool result:\n%s", gemini)
	}

	resp, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, in, "r", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp), "function_call") || !strings.Contains(string(resp), "function_call_output") {
		t.Fatalf("responses tools:\n%s", resp)
	}
}

func TestRequest_ClaudeImageURLAndFileSourceDropped(t *testing.T) {
	in := []byte(`{"model":"c","max_tokens":8,"messages":[{"role":"user","content":[
		{"type":"image","source":{"type":"url","url":"https://example.com/a.png"}},
		{"type":"image","source":{"type":"file","media_type":"image/png","data":"QQ=="}},
		{"type":"document","source":{"type":"base64","data":"JVBERi0="}},
		{"type":"text","text":"see"}
	]}]}`)
	chat, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if !strings.Contains(s, "https://example.com/a.png") {
		t.Fatalf("image URL should keep toward Chat:\n%s", s)
	}
	if strings.Contains(s, `"type":"file"`) && strings.Contains(s, "QQ==") && !strings.Contains(s, "JVBERi0=") {
		t.Fatalf("file source image should drop, document keep:\n%s", s)
	}
	if !strings.Contains(s, "JVBERi0=") {
		t.Fatalf("document bytes dropped:\n%s", s)
	}

	gemini, err := Request(config.ProtocolClaudeMessages, config.ProtocolGemini, in, "g", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(gemini), "example.com") {
		t.Fatalf("HTTP image must drop toward Gemini:\n%s", gemini)
	}
}

func TestRequest_ClaudeToolResultNonArray(t *testing.T) {
	in := []byte(`{"model":"c","max_tokens":8,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":{"ok":true}}]}]}`)
	out, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"role":"tool"`) {
		t.Fatalf("object tool_result:\n%s", out)
	}

	empty := []byte(`{"model":"c","max_tokens":8,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":""}]}]}`)
	out, err = Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, empty, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "t1") {
		t.Fatalf("empty tool_result:\n%s", out)
	}

	bad := []byte(`{"model":"c","max_tokens":8,"messages":[{"role":"user","content":{"not":"blocks"}}]}`)
	out, err = Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, bad, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"role":"user"`) {
		t.Fatalf("non-block content:\n%s", out)
	}
}

func TestResponse_ChatDefaultIDsAndUsage(t *testing.T) {
	in := []byte(`{"candidates":[{"content":{"parts":[{"thought":true,"text":"hmm"},{"text":"pong"},{"functionCall":{"name":"fn"}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":4}}`)
	chat, err := Response(config.ProtocolOpenAIChat, config.ProtocolGemini, in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(chat)
	if !strings.Contains(s, "chatcmpl_") {
		t.Fatalf("default chat id:\n%s", s)
	}
	if !strings.Contains(s, `"prompt_tokens":3`) && !strings.Contains(s, `"prompt_tokens": 3`) {
		t.Fatalf("usage:\n%s", s)
	}
	if !strings.Contains(s, "fn") || !strings.Contains(s, "tool_calls") {
		t.Fatalf("gemini functionCall:\n%s", s)
	}

	resp, err := Response(config.ProtocolOpenAIResponses, config.ProtocolGemini, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resp), `"object":"response"`) || !strings.Contains(string(resp), "pong") {
		t.Fatalf("gemini→responses:\n%s", resp)
	}

	claude, err := Response(config.ProtocolClaudeMessages, config.ProtocolGemini, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(claude), "hmm") || !strings.Contains(string(claude), "pong") {
		t.Fatalf("gemini→claude:\n%s", claude)
	}
}

func TestMustJSONAndParseDataURL(t *testing.T) {
	if got := string(mustJSON("hi")); got != `"hi"` {
		t.Fatalf("mustJSON string=%s", got)
	}
	b := mustJSON(map[string]int{"a": 1})
	if !bytes.Contains(b, []byte(`"a"`)) {
		t.Fatalf("mustJSON object=%s", b)
	}
	mime, data := parseDataURL("not-data")
	if mime != "" || data != "" {
		t.Fatalf("non data url: %q %q", mime, data)
	}
	mime, data = parseDataURL("data:image/png,QQ==")
	if mime != "" || data != "" {
		t.Fatalf("missing base64 marker: %q %q", mime, data)
	}
	if got := encodeDataURL("", "QQ=="); !strings.HasPrefix(got, "data:application/octet-stream;base64,") {
		t.Fatalf("empty mime: %s", got)
	}
}

func TestGeminiPartsTextSkipsThought(t *testing.T) {
	got := geminiPartsText([]geminiPart{{Text: "a", Thought: true}, {Text: "b"}}, false)
	if got != "b" {
		t.Fatalf("skip thought: %q", got)
	}
	got = geminiPartsText([]geminiPart{{Text: "a", Thought: true}, {Text: "b"}}, true)
	if got != "ab" {
		t.Fatalf("include thought: %q", got)
	}
}

func TestFirstNonEmptyAndRawString(t *testing.T) {
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Fatalf("a wins: %q", got)
	}
	if got := firstNonEmpty("", "b"); got != "b" {
		t.Fatalf("b fallback: %q", got)
	}
	if got := rawString(nil); got != "" {
		t.Fatalf("nil raw: %q", got)
	}
	if got := rawString(json.RawMessage(`"hi"`)); got != "hi" {
		t.Fatalf("json string: %q", got)
	}
	if got := rawString(json.RawMessage(`hi`)); got != "hi" {
		t.Fatalf("bare: %q", got)
	}
}

func TestResponsesOutputText(t *testing.T) {
	if got := responsesOutputText(nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
	if got := responsesOutputText(json.RawMessage(`"hello"`)); got != "hello" {
		t.Fatalf("string: %q", got)
	}
	if got := responsesOutputText(json.RawMessage(`[{"text":"a"},{"text":"b"}]`)); got != "ab" {
		t.Fatalf("parts: %q", got)
	}
	if got := responsesOutputText(json.RawMessage(`{"no":1}`)); got != "" {
		t.Fatalf("object: %q", got)
	}
}

func TestChatToGeminiResponseDirect(t *testing.T) {
	in := []byte(`{"choices":[{"message":{"content":"hi","reasoning_content":"think","tool_calls":[{"function":{"name":"fn","arguments":"{}"}}]}}],"usage":{"prompt_tokens":1,"completion_tokens":2}}`)
	out, err := chatToGeminiResponse(in)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "hi") || !strings.Contains(s, "think") || !strings.Contains(s, "fn") {
		t.Fatalf("chatToGeminiResponse:\n%s", s)
	}
	if _, err := chatToGeminiResponse([]byte(`{`)); err == nil {
		t.Fatal("invalid json")
	}
	empty, err := chatToGeminiResponse([]byte(`{"choices":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(empty), `"candidates"`) {
		t.Fatalf("empty choices:\n%s", empty)
	}
}

func TestWriteClaudeOneShotSSE_ErrorAndEmptyID(t *testing.T) {
	var out bytes.Buffer
	if err := writeClaudeOneShotSSE(&out, []byte(`{"type":"error","error":{"type":"api_error","message":"x"}}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "event: error") {
		t.Fatalf("error sse:\n%s", out.String())
	}
	out.Reset()
	if err := writeClaudeOneShotSSE(&out, []byte(`{"type":"message","content":[{"type":"text","text":"hi"}]}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "msg_caosi") || !strings.Contains(out.String(), "hi") {
		t.Fatalf("default id:\n%s", out.String())
	}
	if err := writeClaudeOneShotSSE(io.Discard, []byte(`{`)); err == nil {
		t.Fatal("invalid json")
	}
}

func TestAssembleUpstreamResponse_ChatDeltas(t *testing.T) {
	raw := []byte(strings.Join([]string{
		`data: {"object":"chat.completion.chunk","choices":[{"delta":{"content":"he","reasoning_content":"t"}}]}`,
		`data: {"choices":[{"delta":{"content":"y"}}]}`,
		`data: [DONE]`,
	}, "\n"))
	got := assembleUpstreamResponse(raw)
	if !strings.Contains(string(got), "hey") || !strings.Contains(string(got), `"reasoning_content"`) {
		t.Fatalf("assembled chat:\n%s", got)
	}

	obj := assembleUpstreamResponse([]byte(`{"id":"x"}`))
	if !bytes.Contains(obj, []byte(`"id"`)) {
		t.Fatalf("json object: %s", obj)
	}
	last := assembleUpstreamResponse([]byte("event: ping\ndata: {\"foo\":1}\n\n"))
	if !bytes.Contains(last, []byte(`"foo"`)) {
		t.Fatalf("last json: %s", last)
	}
}

func TestUnwrapEventEnvelope(t *testing.T) {
	inner := unwrapEventEnvelope([]byte(`{"type":"response.completed","response":{"id":"r1"}}`))
	if !bytes.Contains(inner, []byte(`"id":"r1"`)) && !bytes.Contains(inner, []byte(`"id": "r1"`)) {
		t.Fatalf("unwrap: %s", inner)
	}
	raw := []byte(`not-json`)
	if string(unwrapEventEnvelope(raw)) != "not-json" {
		t.Fatalf("invalid: %s", unwrapEventEnvelope(raw))
	}
	same := unwrapEventEnvelope([]byte(`{"type":"response.completed","response":null}`))
	if !bytes.Contains(same, []byte(`response.completed`)) {
		t.Fatalf("null response: %s", same)
	}
}

func TestIRTextThinkingSplit(t *testing.T) {
	parts := []irPart{
		textPart("a"),
		thinkingPart("t"),
		{Kind: irKindToolCall, ID: "c", Name: "n", Args: ""},
		{Kind: irKindToolResult, Nested: []irPart{textPart("b")}},
	}
	if got := irText(parts); got != "ab" {
		t.Fatalf("irText=%q", got)
	}
	if got := irThinking(parts); got != "t" {
		t.Fatalf("irThinking=%q", got)
	}
	rest, calls := splitIRToolCalls(parts)
	if len(calls) != 1 || calls[0].Name != "n" {
		t.Fatalf("calls=%v", calls)
	}
	if irText(rest) != "ab" {
		t.Fatalf("rest text")
	}
	if nestedToolID(parts) != "" {
		t.Fatal("no tool use id")
	}
	if nestedToolID([]irPart{{Kind: irKindToolResult, ToolUseID: "x"}}) != "x" {
		t.Fatal("tool use id")
	}
}

func TestClassifyMIME(t *testing.T) {
	if classifyMIME("Image/PNG") != irKindImage {
		t.Fatal("image")
	}
	if classifyMIME("audio/wav") != irKindAudio {
		t.Fatal("audio")
	}
	if classifyMIME("video/mp4") != irKindVideo {
		t.Fatal("video")
	}
	if classifyMIME("application/pdf") != irKindDocument {
		t.Fatal("document")
	}
}

func TestAudioFormatFromMIME(t *testing.T) {
	if audioFormatFromMIME("audio/x-wav") != "wav" {
		t.Fatal("x-wav")
	}
	if audioFormatFromMIME("audio/mp3") != "mp3" {
		t.Fatal("mp3")
	}
	if audioFormatFromMIME("audio/ogg") != "" {
		t.Fatal("ogg")
	}
}

func TestImageURLToIR_HTTP(t *testing.T) {
	p := imageURLToIR("https://example.com/a.png")
	if p.Kind != irKindImage || p.URL != "https://example.com/a.png" {
		t.Fatalf("%+v", p)
	}
	p = imageURLToIR("data:;base64,QQ==")
	if p.Data != "QQ==" || p.MIME != "image/png" {
		t.Fatalf("empty mime data url: %+v", p)
	}
}

func TestIrPartsToChatContent_AudioAndEmptyImage(t *testing.T) {
	got := irPartsToChatContent([]irPart{
		{Kind: irKindImage},
		{Kind: irKindDocument, Data: "QQ=="},
		{Kind: irKindAudio, Data: "UklGRg==", MIME: "audio/wav"},
		{Kind: irKindAudio, Data: "xx", AudioFmt: "ogg"},
		textPart("hi"),
	})
	b, _ := json.Marshal(got)
	s := string(b)
	if !strings.Contains(s, "input_audio") || !strings.Contains(s, "file") || !strings.Contains(s, "hi") {
		t.Fatalf("chat content: %s", s)
	}
	if strings.Contains(s, "ogg") {
		t.Fatalf("ogg audio must drop: %s", s)
	}
}

func TestIrToClaudeMediaEmptyMIME(t *testing.T) {
	b, ok := irMediaToClaudeBlock("image", irPart{Kind: irKindImage, Data: "QQ=="})
	if !ok || b["source"].(map[string]any)["media_type"] != "image/png" {
		t.Fatalf("default image mime: %#v", b)
	}
	b, ok = irMediaToClaudeBlock("document", irPart{Kind: irKindDocument, Data: "QQ=="})
	if !ok || b["source"].(map[string]any)["media_type"] != "application/octet-stream" {
		t.Fatalf("default doc mime: %#v", b)
	}
	_, ok = irMediaToClaudeBlock("image", irPart{})
	if ok {
		t.Fatal("empty media")
	}
}

func TestIrToGeminiSkipsURLOnlyMedia(t *testing.T) {
	parts := irPartsToGemini([]irPart{
		{Kind: irKindImage, URL: "https://x"},
		{Kind: irKindText, Text: "hi"},
		{Kind: irKindToolCall, Name: "fn"},
		{Kind: irKindThinking},
	})
	if len(parts) != 2 {
		t.Fatalf("parts=%#v", parts)
	}
	if parts[0].Text != "hi" || parts[1].FunctionCall == nil {
		t.Fatalf("unexpected %#v", parts)
	}
}

func TestRequest_StreamIncludeUsage(t *testing.T) {
	in := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`)
	out, err := Request(config.ProtocolOpenAIChat, config.ProtocolOpenAIChat, in, "m", true)
	if err != nil {
		t.Fatal(err)
	}
	// passthrough applyModel does not add stream_options; conversion path does.
	conv, err := Request(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, []byte(`{"model":"c","max_tokens":8,"messages":[{"role":"user","content":"hi"}]}`), "m", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(conv), `"include_usage":true`) && !strings.Contains(string(conv), `"include_usage": true`) {
		t.Fatalf("stream_options:\n%s", conv)
	}
	_ = out
}

func TestResponse_ClaudeInvalidJSONAndGeminiBlobAlt(t *testing.T) {
	_, err := Response(config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, []byte(`{`))
	if err == nil {
		t.Fatal("invalid claude json")
	}
	_, err = Response(config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, []byte(`{`))
	if err == nil {
		t.Fatal("invalid responses json")
	}
	_, err = Response(config.ProtocolOpenAIChat, config.ProtocolGemini, []byte(`{`))
	if err == nil {
		t.Fatal("invalid gemini json")
	}

	in := []byte(`{"candidates":[{"content":{"parts":[{"inline_data":{"data":"QQ=="}}]}}]}`)
	out, err := Response(config.ProtocolGemini, config.ProtocolGemini, in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "QQ==") {
		t.Fatalf("passthrough:\n%s", out)
	}
	chat, err := Response(config.ProtocolGemini, config.ProtocolOpenAIChat, []byte(`{"id":"x","choices":[{"message":{"content":"hi"},"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chat), "hi") {
		t.Fatalf("chat→gemini client:\n%s", chat)
	}

	blob := []byte(`{"candidates":[{"content":{"parts":[{"inline_data":{"data":"QQ==","mimeType":""}}]}}]}`)
	g, err := Response(config.ProtocolGemini, config.ProtocolOpenAIChat, []byte(`{"choices":[{"message":{"content":"x"},"finish_reason":"stop"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = blob
	_ = g
	ir, err := geminiToIRResponse(blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(ir.Parts) == 0 || ir.Parts[0].Data != "QQ==" {
		t.Fatalf("inline_data alt: %+v", ir.Parts)
	}
	if ir.Parts[0].MIME != "image/png" {
		t.Fatalf("default mime %q", ir.Parts[0].MIME)
	}
}

func TestRequest_GeminiAssistantMediaToResponses(t *testing.T) {
	in := []byte(`{"contents":[{"role":"model","parts":[{"text":"caption"},{"inlineData":{"mimeType":"image/png","data":"QQ=="}}]}]}`)
	out, err := Request(config.ProtocolGemini, config.ProtocolOpenAIResponses, in, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"type":"output_text"`) || !strings.Contains(s, "caption") {
		t.Fatalf("assistant text:\n%s", s)
	}
	if !strings.Contains(s, "QQ==") {
		t.Fatalf("assistant image:\n%s", s)
	}
}

func TestIrToClaude_NestedToolResultMedia(t *testing.T) {
	body, err := irToClaudeRequest(irRequest{MaxTokens: 8, Messages: []irMessage{{
		Role:       "tool",
		ToolCallID: "t1",
		Parts: []irPart{{
			Kind:      irKindToolResult,
			ToolUseID: "t1",
			Nested: []irPart{
				textPart("ok"),
				{Kind: irKindImage, Data: "QQ==", MIME: "image/png"},
				{Kind: irKindDocument, Data: "JVBERi0=", MIME: "application/pdf"},
			},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `"type":"tool_result"`) || !strings.Contains(s, "QQ==") || !strings.Contains(s, `"type":"document"`) {
		t.Fatalf("nested media:\n%s", s)
	}
	if !strings.Contains(s, "ok") {
		t.Fatalf("nested text:\n%s", s)
	}
}

func TestIrToResponses_EmptyContentAndImageMIME(t *testing.T) {
	got := irPartsToResponsesContent(nil, "user")
	b, _ := json.Marshal(got)
	if !strings.Contains(string(b), `"input_text"`) {
		t.Fatalf("empty user: %s", b)
	}
	got = irPartsToResponsesContent([]irPart{
		{Kind: irKindImage, Data: "QQ=="},
		{Kind: irKindDocument, Data: "xx"},
		{Kind: irKindDocument},
		textPart("hi"),
	}, "assistant")
	b, _ = json.Marshal(got)
	s := string(b)
	if !strings.Contains(s, `"output_text"`) || !strings.Contains(s, "hi") {
		t.Fatalf("assistant text: %s", s)
	}
	if !strings.Contains(s, "image/png") {
		t.Fatalf("default image mime: %s", s)
	}
	if !strings.Contains(s, "document.bin") {
		t.Fatalf("default filename: %s", s)
	}
}

func TestStream_EmptyChatToClaudeDoesNotStart(t *testing.T) {
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("empty stream should not emit: %s", out.String())
	}
}

func TestStream_WriteErrors(t *testing.T) {
	in := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"x"}}]}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, strings.NewReader(in), errWriter{}); err == nil {
		t.Fatal("want write error")
	}
	if err := Stream(config.ProtocolOpenAIResponses, config.ProtocolOpenAIChat, strings.NewReader(in), errWriter{}); err == nil {
		t.Fatal("want responses write error")
	}
	if err := Stream(config.ProtocolGemini, config.ProtocolOpenAIChat, strings.NewReader(in), errWriter{}); err == nil {
		t.Fatal("want gemini write error")
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestEncodeClaudeErrorDefaultType(t *testing.T) {
	b, err := encodeClaudeError("", "msg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"type":"api_error"`) || !strings.Contains(string(b), "msg") {
		t.Fatalf("%s", b)
	}
}

func TestClaudeSourceURLEmptyAndImageDefaultMIME(t *testing.T) {
	_, ok := claudeSourceToIR(irKindImage, &claudeImgSrc{Type: "url"})
	if ok {
		t.Fatal("empty url")
	}
	_, ok = claudeSourceToIR(irKindImage, &claudeImgSrc{Type: "base64"})
	if ok {
		t.Fatal("empty data")
	}
	p, ok := claudeSourceToIR(irKindImage, &claudeImgSrc{Data: "QQ=="})
	if !ok || p.MIME != "image/png" {
		t.Fatalf("%+v ok=%v", p, ok)
	}
	p, ok = claudeSourceToIR(irKindImage, &claudeImgSrc{URL: "https://x", Data: ""})
	if !ok || p.URL != "https://x" {
		t.Fatalf("url source %+v", p)
	}
}

func TestStream_ResponsesReasoningTextDeltaAndSnapshot(t *testing.T) {
	in := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"r1","model":"m","status":"in_progress"}}`,
		``,
		`event: response.reasoning_text.delta`,
		`data: {"type":"response.reasoning_text.delta","delta":"why"}`,
		``,
		`event: ping`,
		`data: {"type":"response.in_progress"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"r1","status":"completed","output":[{"type":"function_call","name":"fn"}]}}`,
		``,
	}, "\n")
	var out bytes.Buffer
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "why") || !strings.Contains(s, `"type":"thinking_delta"`) {
		t.Fatalf("reasoning_text.delta:\n%s", s)
	}

	complete := `{"object":"response","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}]}`
	out.Reset()
	if err := Stream(config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses, strings.NewReader(complete), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "done") {
		t.Fatalf("complete json:\n%s", out.String())
	}
}

func TestIrToChatResponse_AudioAndTools(t *testing.T) {
	b, err := irToChatResponse(irResponse{
		Parts: []irPart{
			textPart("hi"),
			{Kind: irKindAudio, Data: "UklGRg=="},
			{Kind: irKindToolCall, ID: "c1", Name: "fn"},
		},
		PromptTokens:     1,
		CompletionTokens: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "UklGRg==") || !strings.Contains(s, "fn") || !strings.Contains(s, "chatcmpl_caosi") {
		t.Fatalf("%s", s)
	}
	if !strings.Contains(s, `"finish_reason":"tool_calls"`) {
		t.Fatalf("finish: %s", s)
	}
}

func TestIrToClaudeRequest_EmptyBlocksAndSystem(t *testing.T) {
	b, err := irToClaudeRequest(irRequest{
		Temperature:     ptrFloat(0.1),
		TopP:            ptrFloat(0.2),
		Stop:            []string{"END"},
		ReasoningEffort: "high",
		Messages: []irMessage{
			{Role: "system", Parts: []irPart{textPart("sys")}},
			{Role: "", Parts: nil},
			{Role: "assistant", Parts: []irPart{{Kind: irKindToolCall, ID: "c1", Name: "fn", Args: "not-json"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"system":"sys"`) && !strings.Contains(s, `"system": "sys"`) {
		t.Fatalf("system: %s", s)
	}
	if !strings.Contains(s, `"content":""`) {
		t.Fatalf("empty blocks: %s", s)
	}
	if !strings.Contains(s, `"thinking"`) {
		t.Fatalf("thinking: %s", s)
	}
}

func ptrFloat(v float64) *float64 { return &v }

func TestChatMsgToIR_EmptyRoleAudioAndEmptyToolArgs(t *testing.T) {
	msgs := chatMsgToIR(openaiMsg{
		Content: "hi",
		Audio:   &openaiAudio{Data: "UklGRg=="},
		ToolCalls: []openaiToolCall{{
			ID: "c1",
		}},
	})
	if len(msgs) != 1 || msgs[0].Role != "user" {
		t.Fatalf("%+v", msgs)
	}
	if irText(msgs[0].Parts) != "hi" {
		t.Fatalf("text")
	}
	var hasAudio, hasCall bool
	for _, p := range msgs[0].Parts {
		if p.Kind == irKindAudio && p.Data == "UklGRg==" {
			hasAudio = true
		}
		if p.Kind == irKindToolCall && p.Args == "{}" {
			hasCall = true
		}
	}
	if !hasAudio || !hasCall {
		t.Fatalf("parts=%+v", msgs[0].Parts)
	}
}

func TestLooksLikeCompleteResponsesAndOutputHasFunctionCall(t *testing.T) {
	if looksLikeCompleteResponses(map[string]json.RawMessage{}) {
		t.Fatal("no output")
	}
	m := map[string]json.RawMessage{
		"output": json.RawMessage(`[]`),
		"object": json.RawMessage(`"response"`),
	}
	if !looksLikeCompleteResponses(m) {
		t.Fatal("object response")
	}
	m["object"] = json.RawMessage(`""`)
	m["type"] = json.RawMessage(`"response.completed"`)
	if looksLikeCompleteResponses(m) {
		t.Fatal("response.* events are not complete bodies")
	}
	if outputHasFunctionCall(json.RawMessage(`nope`)) {
		t.Fatal("invalid")
	}
	if !outputHasFunctionCall(json.RawMessage(`[{"type":"custom_tool_call"}]`)) {
		t.Fatal("custom_tool_call")
	}
}

func TestSystemTextNullAndInvalid(t *testing.T) {
	if systemText(nil) != "" || systemText(json.RawMessage(`null`)) != "" {
		t.Fatal("null")
	}
	if systemText(json.RawMessage(`{"x":1}`)) != "" {
		t.Fatal("object")
	}
	if systemText(json.RawMessage(`[{"type":"","text":"a"}]`)) != "a" {
		t.Fatal("empty type")
	}
}

func TestConvertToolChoiceDirect(t *testing.T) {
	if convertToolChoice(nil) != nil || convertToolChoice(json.RawMessage(`null`)) != nil {
		t.Fatal("null")
	}
	if convertToolChoice(json.RawMessage(`{"type":"tool"}`)) != nil {
		t.Fatal("tool without name")
	}
}
