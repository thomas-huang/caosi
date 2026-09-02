package convert

import (
	"encoding/json"
	"fmt"
	"strings"
)

type geminiReq struct {
	Contents          []geminiContent `json:"contents"`
	SystemInstruction *geminiContent  `json:"systemInstruction"`
	Tools             []geminiTool    `json:"tools"`
	GenerationConfig  *geminiGen      `json:"generationConfig"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string        `json:"text,omitempty"`
	Thought          bool          `json:"thought,omitempty"`
	InlineData       *geminiBlob   `json:"inlineData,omitempty"`
	InlineDataAlt    *geminiBlob   `json:"inline_data,omitempty"`
	FunctionCall     *geminiFnCall `json:"functionCall,omitempty"`
	FunctionCallAlt  *geminiFnCall `json:"function_call,omitempty"`
	FunctionResponse *geminiFnResp `json:"functionResponse,omitempty"`
}

type geminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFnCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiFnResp struct {
	Name     string          `json:"name"`
	Response json.RawMessage `json:"response"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFnDecl `json:"functionDeclarations"`
}

type geminiFnDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type geminiGen struct {
	Temperature     *float64 `json:"temperature"`
	TopP            *float64 `json:"topP"`
	MaxOutputTokens int      `json:"maxOutputTokens"`
	ThinkingConfig  *struct {
		ThinkingBudget int    `json:"thinkingBudget"`
		ThinkingLevel  string `json:"thinkingLevel"`
	} `json:"thinkingConfig"`
}

type geminiResp struct {
	Candidates []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func GeminiToOpenAIChat(body []byte, model string, stream bool) ([]byte, error) {
	var in geminiReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Gemini 请求不是合法 JSON: %w", err)
	}
	out := openaiChatReq{Stream: stream, Model: model}
	if out.Model == "" {
		out.Model = "gemini-pro"
	}
	if in.GenerationConfig != nil {
		out.Temperature = in.GenerationConfig.Temperature
		out.TopP = in.GenerationConfig.TopP
		out.MaxTokens = in.GenerationConfig.MaxOutputTokens
		if in.GenerationConfig.ThinkingConfig != nil {
			lvl := in.GenerationConfig.ThinkingConfig.ThinkingLevel
			if lvl == "" {
				lvl = "medium"
			}
			out.ReasoningEffort = strings.ToLower(lvl)
		}
	}
	if stream {
		out.StreamOptions = &streamOpts{IncludeUsage: true}
	}
	if in.SystemInstruction != nil {
		if t := geminiPartsText(in.SystemInstruction.Parts, false); t != "" {
			out.Messages = append(out.Messages, openaiMsg{Role: "system", Content: t})
		}
	}
	for _, c := range in.Contents {
		out.Messages = append(out.Messages, geminiContentToChat(c)...)
	}
	for _, t := range in.Tools {
		for _, d := range t.FunctionDeclarations {
			out.Tools = append(out.Tools, openaiTool{
				Type: "function",
				Function: openaiToolFn{
					Name:        d.Name,
					Description: d.Description,
					Parameters:  d.Parameters,
				},
			})
		}
	}
	return json.Marshal(out)
}

func geminiContentToChat(c geminiContent) []openaiMsg {
	role := "user"
	if c.Role == "model" {
		role = "assistant"
	}
	var parts []openaiPart
	var toolCalls []openaiToolCall
	var toolResults []openaiMsg
	var text strings.Builder
	var thinking strings.Builder
	for _, p := range c.Parts {
		fc := p.FunctionCall
		if fc == nil {
			fc = p.FunctionCallAlt
		}
		blob := p.InlineData
		if blob == nil {
			blob = p.InlineDataAlt
		}
		switch {
		case p.Thought && p.Text != "":
			thinking.WriteString(p.Text)
		case p.Text != "":
			text.WriteString(p.Text)
			parts = append(parts, openaiPart{Type: "text", Text: p.Text})
		case blob != nil && blob.Data != "":
			mt := blob.MimeType
			if mt == "" {
				mt = "image/png"
			}
			parts = append(parts, openaiPart{Type: "image_url", ImageURL: &openaiImgURL{URL: "data:" + mt + ";base64," + blob.Data}})
		case fc != nil:
			args := "{}"
			if len(fc.Args) > 0 {
				args = string(fc.Args)
			}
			tc := openaiToolCall{ID: "call_" + fc.Name, Type: "function"}
			tc.Function.Name = fc.Name
			tc.Function.Arguments = args
			toolCalls = append(toolCalls, tc)
		case p.FunctionResponse != nil:
			content := string(p.FunctionResponse.Response)
			toolResults = append(toolResults, openaiMsg{
				Role:       "tool",
				Name:       p.FunctionResponse.Name,
				ToolCallID: "call_" + p.FunctionResponse.Name,
				Content:    content,
			})
		}
	}
	_ = thinking
	if len(toolResults) > 0 && len(toolCalls) == 0 && text.Len() == 0 {
		return toolResults
	}
	msg := openaiMsg{Role: role}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
		if text.Len() > 0 {
			msg.Content = text.String()
		}
		return append([]openaiMsg{msg}, toolResults...)
	}
	if hasImage(parts) {
		msg.Content = parts
	} else {
		msg.Content = text.String()
	}
	return []openaiMsg{msg}
}

func hasImage(parts []openaiPart) bool {
	for _, p := range parts {
		if p.Type == "image_url" {
			return true
		}
	}
	return false
}

func geminiPartsText(parts []geminiPart, includeThought bool) string {
	var b strings.Builder
	for _, p := range parts {
		if p.Thought && !includeThought {
			continue
		}
		b.WriteString(p.Text)
	}
	return b.String()
}

func OpenAIChatToGemini(body []byte, model string, stream bool) ([]byte, error) {
	var in openaiChatReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Chat 请求不是合法 JSON: %w", err)
	}
	out := geminiReq{}
	gen := &geminiGen{}
	if in.Temperature != nil {
		gen.Temperature = in.Temperature
	}
	if in.TopP != nil {
		gen.TopP = in.TopP
	}
	if in.MaxTokens > 0 {
		gen.MaxOutputTokens = in.MaxTokens
	}
	if in.ReasoningEffort != "" {
		gen.ThinkingConfig = &struct {
			ThinkingBudget int    `json:"thinkingBudget"`
			ThinkingLevel  string `json:"thinkingLevel"`
		}{ThinkingLevel: in.ReasoningEffort}
	}
	out.GenerationConfig = gen
	for _, msg := range in.Messages {
		switch msg.Role {
		case "system":
			out.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: contentAsString(msg.Content)}}}
		case "tool":
			out.Contents = append(out.Contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{
					FunctionResponse: &geminiFnResp{Name: msg.Name, Response: json.RawMessage(mustJSON(msg.Content))},
				}},
			})
		default:
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}
			c := geminiContent{Role: role}
			c.Parts = append(c.Parts, chatContentToGeminiParts(msg.Content)...)
			for _, tc := range msg.ToolCalls {
				c.Parts = append(c.Parts, geminiPart{FunctionCall: &geminiFnCall{
					Name: tc.Function.Name,
					Args: json.RawMessage(tc.Function.Arguments),
				}})
			}
			out.Contents = append(out.Contents, c)
		}
	}
	if len(in.Tools) > 0 {
		var decls []geminiFnDecl
		for _, t := range in.Tools {
			decls = append(decls, geminiFnDecl{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			})
		}
		out.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}
	_ = model
	_ = stream
	return json.Marshal(out)
}

func chatContentToGeminiParts(content any) []geminiPart {
	switch c := content.(type) {
	case string:
		if c == "" {
			return nil
		}
		return []geminiPart{{Text: c}}
	default:
		b, _ := json.Marshal(content)
		var parts []openaiPart
		if json.Unmarshal(b, &parts) == nil {
			var out []geminiPart
			for _, p := range parts {
				if p.Type == "image_url" && p.ImageURL != nil {
					url := p.ImageURL.URL
					mt, data := splitDataURL(url)
					if data != "" {
						out = append(out, geminiPart{InlineData: &geminiBlob{MimeType: mt, Data: data}})
					}
				} else if p.Text != "" {
					out = append(out, geminiPart{Text: p.Text})
				}
			}
			return out
		}
		s := contentAsString(content)
		if s == "" {
			return nil
		}
		return []geminiPart{{Text: s}}
	}
}

func splitDataURL(url string) (mime, data string) {
	if !strings.HasPrefix(url, "data:") {
		return "image/png", ""
	}
	rest := strings.TrimPrefix(url, "data:")
	i := strings.Index(rest, ";base64,")
	if i < 0 {
		return "image/png", ""
	}
	return rest[:i], rest[i+len(";base64,"):]
}

func GeminiToChatResponse(body []byte) ([]byte, error) {
	var in geminiResp
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Gemini 响应不是合法 JSON: %w", err)
	}
	out := openaiChatResp{}
	if in.Error != nil && in.Error.Message != "" {
		out.Error = &struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		}{Message: in.Error.Message, Type: in.Error.Status}
		return json.Marshal(out)
	}
	content := ""
	thinking := ""
	var tcs []openaiToolCall
	finish := "stop"
	if len(in.Candidates) > 0 {
		cand := in.Candidates[0]
		if cand.FinishReason == "MAX_TOKENS" {
			finish = "length"
		}
		for _, p := range cand.Content.Parts {
			if p.Thought {
				thinking += p.Text
				continue
			}
			if p.Text != "" {
				content += p.Text
			}
			fc := p.FunctionCall
			if fc == nil {
				fc = p.FunctionCallAlt
			}
			if fc != nil {
				tc := openaiToolCall{ID: "call_" + fc.Name, Type: "function"}
				tc.Function.Name = fc.Name
				tc.Function.Arguments = string(fc.Args)
				if tc.Function.Arguments == "" {
					tc.Function.Arguments = "{}"
				}
				tcs = append(tcs, tc)
				finish = "tool_calls"
			}
		}
	}
	raw, _ := json.Marshal(map[string]any{
		"id":    "chatcmpl_gemini",
		"model": "gemini",
		"choices": []any{map[string]any{
			"index":         0,
			"finish_reason": finish,
			"message": map[string]any{
				"role":              "assistant",
				"content":           content,
				"reasoning_content": thinking,
				"tool_calls":        tcs,
			},
		}},
	})
	if in.UsageMetadata != nil {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		m["usage"] = map[string]int{
			"prompt_tokens":     in.UsageMetadata.PromptTokenCount,
			"completion_tokens": in.UsageMetadata.CandidatesTokenCount,
		}
		raw, _ = json.Marshal(m)
	}
	return raw, nil
}

func ChatToGeminiResponse(body []byte) ([]byte, error) {
	var in openaiChatResp
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	var parts []geminiPart
	if len(in.Choices) > 0 {
		ch := in.Choices[0]
		if ch.Message.ReasoningContent != "" {
			parts = append(parts, geminiPart{Text: ch.Message.ReasoningContent, Thought: true})
		}
		if ch.Message.Content != "" {
			parts = append(parts, geminiPart{Text: ch.Message.Content})
		}
		for _, tc := range ch.Message.ToolCalls {
			parts = append(parts, geminiPart{FunctionCall: &geminiFnCall{
				Name: tc.Function.Name,
				Args: json.RawMessage(tc.Function.Arguments),
			}})
		}
	}
	out := map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": "STOP",
		}},
	}
	if in.Usage != nil {
		out["usageMetadata"] = map[string]int{
			"promptTokenCount":     in.Usage.PromptTokens,
			"candidatesTokenCount": in.Usage.CompletionTokens,
		}
	}
	return json.Marshal(out)
}

func GeminiToResponses(body []byte, model string, stream bool) ([]byte, error) {
	chat, err := GeminiToOpenAIChat(body, model, stream)
	if err != nil {
		return nil, err
	}
	return ChatToResponses(chat, model, stream)
}

func mustJSON(v any) []byte {
	switch t := v.(type) {
	case string:
		b, _ := json.Marshal(t)
		return b
	default:
		b, _ := json.Marshal(v)
		return b
	}
}
