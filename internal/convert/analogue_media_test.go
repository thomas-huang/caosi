package convert

import (
	"strings"
	"testing"

	"github.com/thomas-huang/caosi/internal/config"
)

const (
	p1Text     = "see-p1"
	p1PNG      = "QQ=="
	p1PDF      = "JVBERi0="
	p1WAV      = "UklGRg=="
	p1MP4      = "AAAAMP4e"
	p1ImgURL   = "https://example.com/p1-img.png"
	p1DocURL   = "https://example.com/p1-doc.pdf"
	p1VidURL   = "https://example.com/p1-vid.mp4"
	p1AudioURL = "https://example.com/p1-a.wav"
)

func p1Req(t *testing.T, src, dst config.Protocol, body []byte) string {
	t.Helper()
	out, err := Request(src, dst, body, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, p1Text) {
		t.Fatalf("text dropped:\n%s", s)
	}
	return s
}

func p1Resp(t *testing.T, client, upstream config.Protocol, body []byte) string {
	t.Helper()
	out, err := Response(client, upstream, body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "pong-p1") {
		t.Fatalf("text dropped:\n%s", s)
	}
	return s
}

func mustHave(t *testing.T, s string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if !strings.Contains(s, n) {
			t.Fatalf("want %q in:\n%s", n, s)
		}
	}
}

func mustOmit(t *testing.T, s string, needles ...string) {
	t.Helper()
	for _, n := range needles {
		if strings.Contains(s, n) {
			t.Fatalf("must drop %q from:\n%s", n, s)
		}
	}
}

func chatParts(parts string) []byte {
	return []byte(`{"model":"gpt-x","messages":[{"role":"user","content":[` + parts + `,{"type":"text","text":"see-p1"}]}]}`)
}

func responsesParts(parts string) []byte {
	return []byte(`{"model":"gpt-x","input":[{"role":"user","content":[` + parts + `,{"type":"input_text","text":"see-p1"}]}]}`)
}

func claudeParts(parts string) []byte {
	return []byte(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":[` + parts + `,{"type":"text","text":"see-p1"}]}]}`)
}

func geminiParts(parts string) []byte {
	return []byte(`{"contents":[{"role":"user","parts":[` + parts + `,{"text":"see-p1"}]}]}`)
}

func TestRequest_AnalogueImageInline(t *testing.T) {
	fixtures := map[config.Protocol][]byte{
		config.ProtocolOpenAIChat:      chatParts(`{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="}}`),
		config.ProtocolOpenAIResponses: responsesParts(`{"type":"input_image","image_url":"data:image/png;base64,QQ=="}`),
		config.ProtocolClaudeMessages:  claudeParts(`{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QQ=="}}`),
		config.ProtocolGemini:          geminiParts(`{"inlineData":{"mimeType":"image/png","data":"QQ=="}}`),
	}
	for src, body := range fixtures {
		for _, dst := range allProtocols {
			if src == dst {
				continue
			}
			t.Run(string(src)+"→"+string(dst), func(t *testing.T) {
				s := p1Req(t, src, dst, body)
				switch dst {
				case config.ProtocolOpenAIChat:
					mustHave(t, s, `"type":"image_url"`, p1PNG)
				case config.ProtocolOpenAIResponses:
					mustHave(t, s, `"type":"input_image"`, p1PNG)
				case config.ProtocolClaudeMessages:
					mustHave(t, s, `"type":"image"`, `"type":"base64"`, p1PNG)
				case config.ProtocolGemini:
					mustHave(t, s, "inlineData", "image/png", p1PNG)
				}
			})
		}
	}
}

func TestRequest_AnalogueImageHTTP(t *testing.T) {
	fixtures := map[config.Protocol][]byte{
		config.ProtocolOpenAIChat:      chatParts(`{"type":"image_url","image_url":{"url":"https://example.com/p1-img.png"}}`),
		config.ProtocolOpenAIResponses: responsesParts(`{"type":"input_image","image_url":"https://example.com/p1-img.png"}`),
		config.ProtocolClaudeMessages:  claudeParts(`{"type":"image","source":{"type":"url","url":"https://example.com/p1-img.png"}}`),
	}
	for src, body := range fixtures {
		for _, dst := range allProtocols {
			if src == dst {
				continue
			}
			t.Run(string(src)+"→"+string(dst), func(t *testing.T) {
				s := p1Req(t, src, dst, body)
				switch dst {
				case config.ProtocolOpenAIChat:
					mustHave(t, s, `"type":"image_url"`, p1ImgURL)
				case config.ProtocolOpenAIResponses:
					mustHave(t, s, `"type":"input_image"`, p1ImgURL)
				case config.ProtocolClaudeMessages:
					mustHave(t, s, `"type":"image"`, `"type":"url"`, p1ImgURL)
				case config.ProtocolGemini:
					mustOmit(t, s, p1ImgURL, "inlineData")
				}
			})
		}
	}
}

func TestRequest_AnalogueDocumentInline(t *testing.T) {
	fixtures := map[config.Protocol][]byte{
		config.ProtocolOpenAIChat:      chatParts(`{"type":"file","file":{"filename":"a.pdf","file_data":"data:application/pdf;base64,JVBERi0="}}`),
		config.ProtocolOpenAIResponses: responsesParts(`{"type":"input_file","filename":"a.pdf","file_data":"data:application/pdf;base64,JVBERi0="}`),
		config.ProtocolClaudeMessages:  claudeParts(`{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}}`),
		config.ProtocolGemini:          geminiParts(`{"inlineData":{"mimeType":"application/pdf","data":"JVBERi0="}}`),
	}
	for src, body := range fixtures {
		for _, dst := range allProtocols {
			if src == dst {
				continue
			}
			t.Run(string(src)+"→"+string(dst), func(t *testing.T) {
				s := p1Req(t, src, dst, body)
				switch dst {
				case config.ProtocolOpenAIChat:
					mustHave(t, s, `"type":"file"`, p1PDF)
					mustOmit(t, s, `"type":"image_url"`)
				case config.ProtocolOpenAIResponses:
					mustHave(t, s, `"type":"input_file"`, p1PDF)
				case config.ProtocolClaudeMessages:
					mustHave(t, s, `"type":"document"`, p1PDF)
					mustOmit(t, s, `"type":"image"`)
				case config.ProtocolGemini:
					mustHave(t, s, "inlineData", "application/pdf", p1PDF)
				}
			})
		}
	}
}

func TestRequest_AnalogueDocumentHTTP(t *testing.T) {
	body := claudeParts(`{"type":"document","source":{"type":"url","url":"https://example.com/p1-doc.pdf"}}`)
	for _, dst := range allProtocols {
		if dst == config.ProtocolClaudeMessages {
			continue
		}
		t.Run("claude→"+string(dst), func(t *testing.T) {
			s := p1Req(t, config.ProtocolClaudeMessages, dst, body)
			switch dst {
			case config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, config.ProtocolGemini:
				mustOmit(t, s, p1DocURL)
			}
		})
	}
}

func TestRequest_AnalogueAudioInline(t *testing.T) {
	fixtures := map[config.Protocol][]byte{
		config.ProtocolOpenAIChat: chatParts(`{"type":"input_audio","input_audio":{"data":"UklGRg==","format":"wav"}}`),
		config.ProtocolGemini:     geminiParts(`{"inlineData":{"mimeType":"audio/wav","data":"UklGRg=="}}`),
	}
	for src, body := range fixtures {
		for _, dst := range allProtocols {
			if src == dst {
				continue
			}
			t.Run(string(src)+"→"+string(dst), func(t *testing.T) {
				s := p1Req(t, src, dst, body)
				switch dst {
				case config.ProtocolOpenAIChat:
					mustHave(t, s, `"type":"input_audio"`, p1WAV)
				case config.ProtocolGemini:
					mustHave(t, s, "inlineData", "audio/wav", p1WAV)
				case config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages:
					mustOmit(t, s, p1WAV)
				}
			})
		}
	}
}

func TestRequest_AnalogueAudioHTTP_Dropped(t *testing.T) {
	body := chatParts(`{"type":"input_audio","input_audio":{"url":"https://example.com/p1-a.wav","format":"wav"}}`)
	for _, dst := range allProtocols {
		if dst == config.ProtocolOpenAIChat {
			continue
		}
		t.Run("chat→"+string(dst), func(t *testing.T) {
			s := p1Req(t, config.ProtocolOpenAIChat, dst, body)
			mustOmit(t, s, p1AudioURL, `"type":"input_audio"`, "inlineData")
		})
	}
}

func TestRequest_AnalogueAudioNonWavMp3_ChatDropsGeminiKeeps(t *testing.T) {
	body := geminiParts(`{"inlineData":{"mimeType":"audio/ogg","data":"T2dnUw=="}}`)
	chat := p1Req(t, config.ProtocolGemini, config.ProtocolOpenAIChat, body)
	mustOmit(t, chat, "T2dnUw==", `"type":"input_audio"`)
	gemini := p1Req(t, config.ProtocolOpenAIChat, config.ProtocolGemini, chatParts(`{"type":"input_audio","input_audio":{"data":"T2dnUw==","format":"ogg"}}`))
	mustHave(t, gemini, "inlineData", "T2dnUw==")
}

func TestRequest_AnalogueVideoInline(t *testing.T) {
	fixtures := map[config.Protocol][]byte{
		config.ProtocolGemini:     geminiParts(`{"inlineData":{"mimeType":"video/mp4","data":"AAAAMP4e"}}`),
		config.ProtocolOpenAIChat: chatParts(`{"type":"video_url","video_url":{"url":"data:video/mp4;base64,AAAAMP4e"}}`),
	}
	for src, body := range fixtures {
		for _, dst := range allProtocols {
			if src == dst {
				continue
			}
			t.Run(string(src)+"→"+string(dst), func(t *testing.T) {
				s := p1Req(t, src, dst, body)
				mustOmit(t, s, `"video_url"`)
				switch dst {
				case config.ProtocolGemini:
					mustHave(t, s, "inlineData", "video/mp4", p1MP4)
				default:
					mustOmit(t, s, p1MP4)
				}
			})
		}
	}
}

func TestRequest_AnalogueVideoHTTP_Dropped(t *testing.T) {
	body := chatParts(`{"type":"video_url","video_url":{"url":"https://example.com/p1-vid.mp4"}}`)
	for _, dst := range allProtocols {
		if dst == config.ProtocolOpenAIChat {
			continue
		}
		t.Run("chat→"+string(dst), func(t *testing.T) {
			s := p1Req(t, config.ProtocolOpenAIChat, dst, body)
			mustOmit(t, s, p1VidURL, `"video_url"`)
		})
	}
}

func TestRequest_AnalogueFileIDAndFileURI_Dropped(t *testing.T) {
	t.Run("chat file_id", func(t *testing.T) {
		s := p1Req(t, config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, chatParts(`{"type":"file","file":{"file_id":"file-p1"}}`))
		mustOmit(t, s, "file-p1")
	})
	t.Run("responses file_id", func(t *testing.T) {
		s := p1Req(t, config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages, responsesParts(`{"type":"input_file","file_id":"file-p1"}`))
		mustOmit(t, s, "file-p1")
	})
	t.Run("claude source.file", func(t *testing.T) {
		s := p1Req(t, config.ProtocolClaudeMessages, config.ProtocolOpenAIChat, claudeParts(`{"type":"document","source":{"type":"file","file_id":"file_p1"}}`))
		mustOmit(t, s, "file_p1")
	})
	t.Run("gemini fileUri", func(t *testing.T) {
		s := p1Req(t, config.ProtocolGemini, config.ProtocolOpenAIChat, geminiParts(`{"fileData":{"mimeType":"application/pdf","fileUri":"https://generativelanguage.googleapis.com/files/p1"}}`))
		mustOmit(t, s, "generativelanguage.googleapis.com", "files/p1")
	})
}

func TestRequest_AnalogueChatAudioDataAndID(t *testing.T) {
	withData := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":"see-p1","audio":{"data":"UklGRg=="}}]}`)
	for _, dst := range []config.Protocol{config.ProtocolGemini, config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses} {
		s := p1Req(t, config.ProtocolOpenAIChat, dst, withData)
		t.Run("data→"+string(dst), func(t *testing.T) {
			switch dst {
			case config.ProtocolGemini:
				mustHave(t, s, "inlineData", p1WAV)
			default:
				mustOmit(t, s, p1WAV)
			}
		})
	}
	idOnly := []byte(`{"model":"gpt-x","messages":[{"role":"user","content":"see-p1","audio":{"id":"aud_p1"}}]}`)
	for _, dst := range []config.Protocol{config.ProtocolGemini, config.ProtocolClaudeMessages, config.ProtocolOpenAIResponses} {
		s := p1Req(t, config.ProtocolOpenAIChat, dst, idOnly)
		t.Run("id→"+string(dst), func(t *testing.T) {
			mustOmit(t, s, "aud_p1")
		})
	}
}

func TestRequest_AnalogueToolResultNestedMedia(t *testing.T) {
	claude := []byte(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_p1","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"QQ=="}},{"type":"text","text":"see-p1"}]}]}]}`)
	for _, dst := range []config.Protocol{config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, config.ProtocolGemini} {
		t.Run("claude→"+string(dst), func(t *testing.T) {
			s := p1Req(t, config.ProtocolClaudeMessages, dst, claude)
			mustOmit(t, s, p1PNG, `"type":"image"`)
		})
	}

	chatTool := []byte(`{"model":"gpt-x","messages":[{"role":"tool","tool_call_id":"call_p1","content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,QQ=="}},{"type":"text","text":"see-p1"}]}]}`)
	t.Run("chat nested image→claude keep", func(t *testing.T) {
		keep := p1Req(t, config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, chatTool)
		mustHave(t, keep, `"type":"tool_result"`, `"type":"image"`, p1PNG, p1Text)
	})
	t.Run("chat nested image→gemini drop", func(t *testing.T) {
		drop := p1Req(t, config.ProtocolOpenAIChat, config.ProtocolGemini, chatTool)
		mustOmit(t, drop, p1PNG)
	})

	claudeDoc := []byte(`{"model":"claude-opus","max_tokens":16,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_p1","content":[{"type":"document","source":{"type":"base64","media_type":"application/pdf","data":"JVBERi0="}},{"type":"text","text":"see-p1"}]}]}]}`)
	for _, dst := range []config.Protocol{config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, config.ProtocolGemini} {
		t.Run("claude nested document→"+string(dst), func(t *testing.T) {
			s := p1Req(t, config.ProtocolClaudeMessages, dst, claudeDoc)
			mustOmit(t, s, p1PDF, `"type":"document"`, `"type":"file"`, `"type":"input_file"`)
		})
	}
	chatDoc := []byte(`{"model":"gpt-x","messages":[{"role":"tool","tool_call_id":"call_p1","content":[{"type":"file","file":{"filename":"a.pdf","file_data":"data:application/pdf;base64,JVBERi0="}},{"type":"text","text":"see-p1"}]}]}`)
	t.Run("chat nested document→claude keep", func(t *testing.T) {
		keep := p1Req(t, config.ProtocolOpenAIChat, config.ProtocolClaudeMessages, chatDoc)
		mustHave(t, keep, `"type":"tool_result"`, `"type":"document"`, p1PDF, p1Text)
	})
}

func TestResponse_AnalogueAssistantMedia(t *testing.T) {
	geminiImg := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"pong-p1"},{"inlineData":{"mimeType":"image/png","data":"QQ=="}}]},"finishReason":"STOP"}]}`)
	for _, client := range allProtocols {
		if client == config.ProtocolGemini {
			continue
		}
		t.Run("image→"+string(client), func(t *testing.T) {
			s := p1Resp(t, client, config.ProtocolGemini, geminiImg)
			mustOmit(t, s, p1PNG)
		})
	}
	keepImg := p1Resp(t, config.ProtocolGemini, config.ProtocolOpenAIChat, []byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"pong-p1"},"finish_reason":"stop"}]}`))
	if !strings.Contains(keepImg, "pong-p1") {
		t.Fatalf("gemini client text:\n%s", keepImg)
	}
	back := p1Resp(t, config.ProtocolGemini, config.ProtocolGemini, geminiImg)
	mustHave(t, back, p1PNG)

	geminiAudio := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"pong-p1"},{"inlineData":{"mimeType":"audio/wav","data":"UklGRg=="}}]},"finishReason":"STOP"}]}`)
	chat := p1Resp(t, config.ProtocolOpenAIChat, config.ProtocolGemini, geminiAudio)
	mustHave(t, chat, `"data":"UklGRg=="`)
	claude := p1Resp(t, config.ProtocolClaudeMessages, config.ProtocolGemini, geminiAudio)
	mustOmit(t, claude, p1WAV)
	resp := p1Resp(t, config.ProtocolOpenAIResponses, config.ProtocolGemini, geminiAudio)
	mustOmit(t, resp, p1WAV)
	t.Run("audio→gemini from chat", func(t *testing.T) {
		chatAudio := []byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"pong-p1","audio":{"data":"UklGRg=="}},"finish_reason":"stop"}]}`)
		s := p1Resp(t, config.ProtocolGemini, config.ProtocolOpenAIChat, chatAudio)
		mustHave(t, s, "inlineData", "audio/wav", p1WAV)
	})
	t.Run("audio→gemini passthrough", func(t *testing.T) {
		s := p1Resp(t, config.ProtocolGemini, config.ProtocolGemini, geminiAudio)
		mustHave(t, s, p1WAV)
	})

	geminiDoc := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"pong-p1"},{"inlineData":{"mimeType":"application/pdf","data":"JVBERi0="}}]},"finishReason":"STOP"}]}`)
	for _, client := range []config.Protocol{config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages} {
		s := p1Resp(t, client, config.ProtocolGemini, geminiDoc)
		t.Run("doc→"+string(client), func(t *testing.T) {
			mustOmit(t, s, p1PDF)
		})
	}
	gdoc := p1Resp(t, config.ProtocolGemini, config.ProtocolGemini, geminiDoc)
	mustHave(t, gdoc, p1PDF, "application/pdf")

	geminiVid := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"pong-p1"},{"inlineData":{"mimeType":"video/mp4","data":"AAAAMP4e"}}]},"finishReason":"STOP"}]}`)
	for _, client := range []config.Protocol{config.ProtocolOpenAIChat, config.ProtocolOpenAIResponses, config.ProtocolClaudeMessages} {
		s := p1Resp(t, client, config.ProtocolGemini, geminiVid)
		t.Run("vid→"+string(client), func(t *testing.T) {
			mustOmit(t, s, p1MP4, `"video_url"`)
		})
	}
	gvid := p1Resp(t, config.ProtocolGemini, config.ProtocolGemini, geminiVid)
	mustHave(t, gvid, p1MP4, "video/mp4")
}

func TestRequest_AnalogueResponsesDocumentURL_Dropped(t *testing.T) {
	body := responsesParts(`{"type":"input_file","file_url":"https://example.com/p1-doc.pdf"}`)
	for _, dst := range []config.Protocol{config.ProtocolClaudeMessages, config.ProtocolGemini, config.ProtocolOpenAIChat} {
		s := p1Req(t, config.ProtocolOpenAIResponses, dst, body)
		mustOmit(t, s, p1DocURL)
	}
}
