package convert

import (
	"encoding/json"
	"fmt"
	"strings"
)

func geminiToIRRequest(body []byte, model string, stream bool) (irRequest, error) {
	var in geminiReq
	if err := json.Unmarshal(body, &in); err != nil {
		return irRequest{}, fmt.Errorf("Gemini 请求不是合法 JSON: %w", err)
	}
	out := irRequest{Stream: stream, IncludeUsage: stream, Model: model}
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
	if in.SystemInstruction != nil {
		if t := geminiPartsText(in.SystemInstruction.Parts, false); t != "" {
			out.Messages = append(out.Messages, irMessage{Role: "system", Parts: []irPart{textPart(t)}})
		}
	}
	for _, c := range in.Contents {
		out.Messages = append(out.Messages, geminiContentToIR(c)...)
	}
	for _, t := range in.Tools {
		for _, d := range t.FunctionDeclarations {
			out.Tools = append(out.Tools, irTool{Name: d.Name, Description: d.Description, Parameters: d.Parameters})
		}
	}
	return out, nil
}

func geminiContentToIR(c geminiContent) []irMessage {
	role := "user"
	if c.Role == "model" {
		role = "assistant"
	}
	var main irMessage
	main.Role = role
	var extra []irMessage
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
			main.Parts = append(main.Parts, thinkingPart(p.Text))
		case p.Text != "":
			main.Parts = append(main.Parts, textPart(p.Text))
		case blob != nil && blob.Data != "":
			mt := blob.MimeType
			if mt == "" {
				mt = "image/png"
			}
			main.Parts = append(main.Parts, mediaPart(classifyMIME(mt), mt, blob.Data, "", ""))
		case fc != nil:
			args := "{}"
			if len(fc.Args) > 0 {
				args = string(fc.Args)
			}
			main.Parts = append(main.Parts, irPart{Kind: irKindToolCall, ID: "call_" + fc.Name, Name: fc.Name, Args: args})
		case p.FunctionResponse != nil:
			content := string(p.FunctionResponse.Response)
			extra = append(extra, irMessage{
				Role:       "tool",
				Name:       p.FunctionResponse.Name,
				ToolCallID: "call_" + p.FunctionResponse.Name,
				Parts:      []irPart{{Kind: irKindToolResult, ToolUseID: "call_" + p.FunctionResponse.Name, Nested: []irPart{textPart(content)}}},
			})
		}
	}
	if len(extra) > 0 && len(main.Parts) == 0 {
		return extra
	}
	if len(main.Parts) == 0 {
		return []irMessage{{Role: role}}
	}
	return append([]irMessage{main}, extra...)
}

func irToGeminiRequest(ir irRequest) ([]byte, error) {
	out := geminiReq{}
	gen := &geminiGen{}
	if ir.Temperature != nil {
		gen.Temperature = ir.Temperature
	}
	if ir.TopP != nil {
		gen.TopP = ir.TopP
	}
	if ir.MaxTokens > 0 {
		gen.MaxOutputTokens = ir.MaxTokens
	}
	if ir.ReasoningEffort != "" {
		gen.ThinkingConfig = &struct {
			ThinkingBudget int    `json:"thinkingBudget"`
			ThinkingLevel  string `json:"thinkingLevel"`
		}{ThinkingLevel: ir.ReasoningEffort}
	}
	out.GenerationConfig = gen
	for _, msg := range ir.Messages {
		switch msg.Role {
		case "system":
			out.SystemInstruction = &geminiContent{Parts: []geminiPart{{Text: irText(msg.Parts)}}}
		case "tool":
			name := msg.Name
			if name == "" {
				name = strings.TrimPrefix(firstNonEmpty(msg.ToolCallID, nestedToolID(msg.Parts)), "call_")
			}
			out.Contents = append(out.Contents, geminiContent{
				Role: "user",
				Parts: []geminiPart{{
					FunctionResponse: &geminiFnResp{Name: name, Response: json.RawMessage(mustJSON(irText(msg.Parts)))},
				}},
			})
		default:
			role := "user"
			if msg.Role == "assistant" {
				role = "model"
			}
			c := geminiContent{Role: role, Parts: irPartsToGemini(msg.Parts)}
			out.Contents = append(out.Contents, c)
		}
	}
	if len(ir.Tools) > 0 {
		var decls []geminiFnDecl
		for _, t := range ir.Tools {
			decls = append(decls, geminiFnDecl{Name: t.Name, Description: t.Description, Parameters: t.Parameters})
		}
		out.Tools = []geminiTool{{FunctionDeclarations: decls}}
	}
	return json.Marshal(out)
}

func irPartsToGemini(parts []irPart) []geminiPart {
	var out []geminiPart
	for _, p := range parts {
		switch p.Kind {
		case irKindThinking:
			if p.Text != "" {
				out = append(out, geminiPart{Text: p.Text, Thought: true})
			}
		case irKindText:
			if p.Text != "" {
				out = append(out, geminiPart{Text: p.Text})
			}
		case irKindImage, irKindDocument, irKindAudio, irKindVideo:
			if p.Data == "" {
				continue
			}
			mime := p.MIME
			if mime == "" {
				mime = "application/octet-stream"
			}
			out = append(out, geminiPart{InlineData: &geminiBlob{MimeType: mime, Data: p.Data}})
		case irKindToolCall:
			args := p.Args
			if args == "" {
				args = "{}"
			}
			out = append(out, geminiPart{FunctionCall: &geminiFnCall{Name: p.Name, Args: json.RawMessage(args)}})
		}
	}
	return out
}

func geminiToIRResponse(body []byte) (irResponse, error) {
	var in geminiResp
	if err := json.Unmarshal(body, &in); err != nil {
		return irResponse{}, fmt.Errorf("Gemini 响应不是合法 JSON: %w", err)
	}
	if in.Error != nil && in.Error.Message != "" {
		return irResponse{ErrorMessage: in.Error.Message, ErrorType: in.Error.Status}, nil
	}
	out := irResponse{ID: "chatcmpl_gemini", Model: "gemini", FinishReason: "stop"}
	if in.UsageMetadata != nil {
		out.PromptTokens = in.UsageMetadata.PromptTokenCount
		out.CompletionTokens = in.UsageMetadata.CandidatesTokenCount
	}
	if len(in.Candidates) == 0 {
		return out, nil
	}
	cand := in.Candidates[0]
	if cand.FinishReason == "MAX_TOKENS" {
		out.FinishReason = "length"
	}
	for _, p := range cand.Content.Parts {
		if p.Thought {
			out.Parts = append(out.Parts, thinkingPart(p.Text))
			continue
		}
		if p.Text != "" {
			out.Parts = append(out.Parts, textPart(p.Text))
		}
		blob := p.InlineData
		if blob == nil {
			blob = p.InlineDataAlt
		}
		if blob != nil && blob.Data != "" {
			mt := blob.MimeType
			if mt == "" {
				mt = "image/png"
			}
			out.Parts = append(out.Parts, mediaPart(classifyMIME(mt), mt, blob.Data, "", ""))
		}
		fc := p.FunctionCall
		if fc == nil {
			fc = p.FunctionCallAlt
		}
		if fc != nil {
			args := string(fc.Args)
			if args == "" {
				args = "{}"
			}
			out.Parts = append(out.Parts, irPart{Kind: irKindToolCall, ID: "call_" + fc.Name, Name: fc.Name, Args: args})
			out.FinishReason = "tool_calls"
		}
	}
	return out, nil
}

func irToGeminiResponse(ir irResponse) ([]byte, error) {
	if ir.ErrorMessage != "" {
		return encodeGeminiError(ir.ErrorMessage, 400), nil
	}
	parts := irPartsToGemini(ir.Parts)
	out := map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"role": "model", "parts": parts},
			"finishReason": "STOP",
		}},
	}
	if ir.PromptTokens != 0 || ir.CompletionTokens != 0 {
		out["usageMetadata"] = map[string]int{
			"promptTokenCount":     ir.PromptTokens,
			"candidatesTokenCount": ir.CompletionTokens,
		}
	}
	return json.Marshal(out)
}
