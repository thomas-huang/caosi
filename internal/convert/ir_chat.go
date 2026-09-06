package convert

import (
	"encoding/json"
	"fmt"
	"strings"
)

func chatToIRRequest(body []byte, model string, stream bool) (irRequest, error) {
	var in openaiChatReq
	if err := json.Unmarshal(body, &in); err != nil {
		return irRequest{}, fmt.Errorf("Chat 请求不是合法 JSON: %w", err)
	}
	out := irRequest{
		Stream:          stream,
		Model:           in.Model,
		MaxTokens:       in.MaxTokens,
		Temperature:     in.Temperature,
		TopP:            in.TopP,
		Stop:            in.Stop,
		ToolChoice:      in.ToolChoice,
		ReasoningEffort: in.ReasoningEffort,
		IncludeUsage:    stream,
	}
	if model != "" {
		out.Model = model
	}
	for _, t := range in.Tools {
		out.Tools = append(out.Tools, irTool{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		})
	}
	for _, m := range in.Messages {
		out.Messages = append(out.Messages, chatMsgToIR(m)...)
	}
	return out, nil
}

func chatMsgToIR(m openaiMsg) []irMessage {
	role := m.Role
	if role == "" {
		role = "user"
	}
	msg := irMessage{Role: role, ToolCallID: m.ToolCallID, Name: m.Name}
	if m.ReasoningContent != "" {
		msg.Parts = append(msg.Parts, thinkingPart(m.ReasoningContent))
	}
	msg.Parts = append(msg.Parts, chatContentToIRParts(m.Content)...)
	for _, tc := range m.ToolCalls {
		args := tc.Function.Arguments
		if args == "" {
			args = "{}"
		}
		msg.Parts = append(msg.Parts, irPart{
			Kind: irKindToolCall,
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		})
	}
	if m.Audio != nil && m.Audio.Data != "" {
		msg.Parts = append(msg.Parts, irPart{
			Kind:     irKindAudio,
			Data:     m.Audio.Data,
			AudioFmt: "wav",
			MIME:     "audio/wav",
		})
	}
	if role == "tool" {
		msg.Parts = []irPart{{Kind: irKindToolResult, ToolUseID: m.ToolCallID, Nested: chatContentToIRParts(m.Content)}}
		if irText(msg.Parts[0].Nested) == "" && len(msg.Parts[0].Nested) == 0 {
			msg.Parts[0].Nested = []irPart{textPart(contentAsString(m.Content))}
		}
	}
	return []irMessage{msg}
}

func chatContentToIRParts(content any) []irPart {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []irPart{textPart(c)}
	case nil:
		return nil
	default:
		b, _ := json.Marshal(c)
		var maps []map[string]any
		if json.Unmarshal(b, &maps) != nil {
			s := contentAsString(c)
			if s == "" {
				return nil
			}
			return []irPart{textPart(s)}
		}
		var parts []irPart
		for _, m := range maps {
			if p, ok := chatMapToIRPart(m); ok {
				parts = append(parts, p)
			}
		}
		return parts
	}
}

func chatMapToIRPart(m map[string]any) (irPart, bool) {
	typ, _ := m["type"].(string)
	switch typ {
	case "text":
		t, _ := m["text"].(string)
		return textPart(t), t != ""
	case "image_url", "image":
		url := chatImageURL(m)
		if url == "" {
			return irPart{}, false
		}
		return imageURLToIR(url), true
	case "file":
		file, _ := m["file"].(map[string]any)
		if file == nil {
			return irPart{}, false
		}
		if _, hasID := file["file_id"].(string); hasID && file["file_data"] == nil {
			return irPart{}, false
		}
		data, _ := file["file_data"].(string)
		name, _ := file["filename"].(string)
		if data == "" {
			return irPart{}, false
		}
		mime, raw := parseDataURL(data)
		if raw == "" {
			raw = data
			mime = "application/octet-stream"
		}
		if mime == "" {
			mime = "application/octet-stream"
		}
		return mediaPart(irKindDocument, mime, raw, "", name), true
	case "input_audio":
		audio, _ := m["input_audio"].(map[string]any)
		if audio == nil {
			return irPart{}, false
		}
		data, _ := audio["data"].(string)
		format, _ := audio["format"].(string)
		if data == "" {
			return irPart{}, false
		}
		mime := mimeFromAudioFormat(format)
		if mime == "" {
			mime = "audio/mpeg"
		}
		return irPart{Kind: irKindAudio, Data: data, AudioFmt: strings.ToLower(format), MIME: mime}, true
	case "video_url":
		url := ""
		if vu, ok := m["video_url"].(map[string]any); ok {
			url, _ = vu["url"].(string)
		} else {
			url, _ = m["video_url"].(string)
		}
		if url == "" {
			return irPart{}, false
		}
		mime, data := parseDataURL(url)
		if data != "" {
			if mime == "" {
				mime = "video/mp4"
			}
			return mediaPart(irKindVideo, mime, data, "", ""), true
		}
		if isHTTPURL(url) {
			return mediaPart(irKindVideo, "video/mp4", "", url, ""), true
		}
		return irPart{}, false
	default:
		if t, _ := m["text"].(string); t != "" && typ == "" {
			return textPart(t), true
		}
		return irPart{}, false
	}
}

func chatImageURL(m map[string]any) string {
	switch iu := m["image_url"].(type) {
	case string:
		return iu
	case map[string]any:
		u, _ := iu["url"].(string)
		return u
	}
	return ""
}

func imageURLToIR(url string) irPart {
	mime, data := parseDataURL(url)
	if data != "" {
		if mime == "" {
			mime = "image/png"
		}
		return mediaPart(irKindImage, mime, data, "", "")
	}
	return mediaPart(irKindImage, "", "", url, "")
}

func irToChatRequest(ir irRequest) ([]byte, error) {
	out := openaiChatReq{
		Model:           ir.Model,
		MaxTokens:       ir.MaxTokens,
		Temperature:     ir.Temperature,
		TopP:            ir.TopP,
		Stream:          ir.Stream,
		Stop:            ir.Stop,
		ToolChoice:      ir.ToolChoice,
		ReasoningEffort: ir.ReasoningEffort,
	}
	if ir.Stream && ir.IncludeUsage {
		out.StreamOptions = &streamOpts{IncludeUsage: true}
	}
	for _, t := range ir.Tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out.Tools = append(out.Tools, openaiTool{
			Type: "function",
			Function: openaiToolFn{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	for _, m := range ir.Messages {
		out.Messages = append(out.Messages, irMsgToChat(m))
	}
	return json.Marshal(out)
}

func irMsgToChat(m irMessage) openaiMsg {
	if m.Role == "tool" {
		return openaiMsg{Role: "tool", ToolCallID: firstNonEmpty(m.ToolCallID, nestedToolID(m.Parts)), Name: m.Name, Content: irText(m.Parts)}
	}
	rest, calls := splitIRToolCalls(m.Parts)
	msg := openaiMsg{Role: m.Role, Name: m.Name, ReasoningContent: irThinking(rest)}
	var contentParts []irPart
	for _, p := range rest {
		if p.Kind == irKindThinking {
			continue
		}
		contentParts = append(contentParts, p)
	}
	msg.Content = irPartsToChatContent(contentParts)
	for _, c := range calls {
		tc := openaiToolCall{ID: c.ID, Type: "function"}
		tc.Function.Name = c.Name
		tc.Function.Arguments = c.Args
		if tc.Function.Arguments == "" {
			tc.Function.Arguments = "{}"
		}
		msg.ToolCalls = append(msg.ToolCalls, tc)
	}
	return msg
}

func nestedToolID(parts []irPart) string {
	for _, p := range parts {
		if p.Kind == irKindToolResult && p.ToolUseID != "" {
			return p.ToolUseID
		}
	}
	return ""
}

func irPartsToChatContent(parts []irPart) any {
	var chatParts []map[string]any
	var text strings.Builder
	hasMedia := false
	for _, p := range parts {
		switch p.Kind {
		case irKindText:
			text.WriteString(p.Text)
			chatParts = append(chatParts, map[string]any{"type": "text", "text": p.Text})
		case irKindImage:
			url := p.URL
			if p.Data != "" {
				mime := p.MIME
				if mime == "" {
					mime = "image/png"
				}
				url = encodeDataURL(mime, p.Data)
			}
			if url == "" {
				continue
			}
			hasMedia = true
			chatParts = append(chatParts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": url}})
		case irKindDocument:
			if p.Data == "" {
				continue
			}
			hasMedia = true
			fn := p.Filename
			if fn == "" {
				fn = filenameFromMIME(p.MIME)
			}
			mime := p.MIME
			if mime == "" {
				mime = "application/octet-stream"
			}
			chatParts = append(chatParts, map[string]any{
				"type": "file",
				"file": map[string]any{"filename": fn, "file_data": encodeDataURL(mime, p.Data)},
			})
		case irKindAudio:
			format := p.AudioFmt
			if format == "" {
				format = audioFormatFromMIME(p.MIME)
			}
			if p.Data == "" || (format != "wav" && format != "mp3") {
				continue
			}
			hasMedia = true
			chatParts = append(chatParts, map[string]any{
				"type":        "input_audio",
				"input_audio": map[string]any{"data": p.Data, "format": format},
			})
		}
	}
	if !hasMedia {
		return text.String()
	}
	return chatParts
}

func chatToIRResponse(body []byte) (irResponse, error) {
	var in openaiChatResp
	if err := json.Unmarshal(body, &in); err != nil {
		return irResponse{}, fmt.Errorf("OpenAI 响应不是合法 JSON: %w", err)
	}
	if in.Error != nil && in.Error.Message != "" {
		return irResponse{ErrorMessage: in.Error.Message, ErrorType: in.Error.Type}, nil
	}
	out := irResponse{ID: in.ID, Model: in.Model}
	if in.Usage != nil {
		out.PromptTokens = in.Usage.PromptTokens
		out.CompletionTokens = in.Usage.CompletionTokens
	}
	if len(in.Choices) == 0 {
		out.FinishReason = "stop"
		return out, nil
	}
	ch := in.Choices[0]
	out.FinishReason = ch.FinishReason
	if ch.Message.ReasoningContent != "" {
		out.Parts = append(out.Parts, thinkingPart(ch.Message.ReasoningContent))
	}
	if ch.Message.Content != "" {
		out.Parts = append(out.Parts, textPart(ch.Message.Content))
	}
	for _, tc := range ch.Message.ToolCalls {
		args := tc.Function.Arguments
		if args == "" {
			args = "{}"
		}
		out.Parts = append(out.Parts, irPart{Kind: irKindToolCall, ID: tc.ID, Name: tc.Function.Name, Args: args})
	}
	if in.Choices[0].Message.Audio != nil && in.Choices[0].Message.Audio.Data != "" {
		out.Parts = append(out.Parts, irPart{Kind: irKindAudio, Data: in.Choices[0].Message.Audio.Data, AudioFmt: "wav", MIME: "audio/wav"})
	}
	return out, nil
}

func irToChatResponse(ir irResponse) ([]byte, error) {
	if ir.ErrorMessage != "" {
		return encodeOpenAIError(ir.ErrorMessage), nil
	}
	id := ir.ID
	if id == "" {
		id = "chatcmpl_caosi"
	}
	finish := ir.FinishReason
	if finish == "" {
		finish = "stop"
	}
	text := irText(ir.Parts)
	thinking := irThinking(ir.Parts)
	var tcs []openaiToolCall
	var audioData string
	for _, p := range ir.Parts {
		if p.Kind == irKindToolCall {
			tc := openaiToolCall{ID: p.ID, Type: "function"}
			tc.Function.Name = p.Name
			tc.Function.Arguments = p.Args
			if tc.Function.Arguments == "" {
				tc.Function.Arguments = "{}"
			}
			tcs = append(tcs, tc)
			if finish == "stop" || finish == "end_turn" {
				finish = "tool_calls"
			}
		}
		if p.Kind == irKindAudio && p.Data != "" {
			audioData = p.Data
		}
	}
	msg := map[string]any{
		"role":              "assistant",
		"content":           text,
		"reasoning_content": thinking,
		"tool_calls":        tcs,
	}
	if audioData != "" {
		msg["audio"] = map[string]any{"data": audioData}
	}
	m := map[string]any{
		"id":    id,
		"model": ir.Model,
		"choices": []any{map[string]any{
			"index":         0,
			"finish_reason": finish,
			"message":       msg,
		}},
	}
	if ir.PromptTokens != 0 || ir.CompletionTokens != 0 {
		m["usage"] = map[string]int{"prompt_tokens": ir.PromptTokens, "completion_tokens": ir.CompletionTokens}
	}
	return json.Marshal(m)
}
