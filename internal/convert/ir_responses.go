package convert

import (
	"encoding/json"
	"fmt"
	"strings"
)

func responsesToIRRequest(body []byte, model string, stream bool) (irRequest, error) {
	var in responsesReq
	if err := json.Unmarshal(body, &in); err != nil {
		return irRequest{}, fmt.Errorf("Responses 请求不是合法 JSON: %w", err)
	}
	out := irRequest{
		Stream:       stream,
		MaxTokens:    in.MaxOutputTokens,
		Temperature:  in.Temperature,
		TopP:         in.TopP,
		ToolChoice:   in.ToolChoice,
		IncludeUsage: stream,
	}
	if model != "" {
		out.Model = model
	} else {
		out.Model = in.Model
	}
	if in.Reasoning != nil && in.Reasoning.Effort != "" {
		out.ReasoningEffort = in.Reasoning.Effort
	}
	if in.Instructions != "" {
		out.Messages = append(out.Messages, irMessage{Role: "system", Parts: []irPart{textPart(in.Instructions)}})
	}
	out.Messages = append(out.Messages, responsesInputToIR(in.Input)...)
	for _, t := range in.Tools {
		fn := openaiToolFn{Name: t.Name, Description: t.Description, Parameters: t.Parameters}
		if t.Function != nil {
			fn = *t.Function
		}
		if fn.Name == "" {
			continue
		}
		out.Tools = append(out.Tools, irTool{Name: fn.Name, Description: fn.Description, Parameters: fn.Parameters})
	}
	return out, nil
}

func responsesInputToIR(raw json.RawMessage) []irMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		return []irMessage{{Role: "user", Parts: []irPart{textPart(asString)}}}
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var msgs []irMessage
	for _, it := range items {
		typ := rawString(it["type"])
		role := rawString(it["role"])
		switch typ {
		case "", "message", "input_text":
			if role == "" {
				role = "user"
			}
			parts := responsesContentToIR(it["content"])
			if role == "system" {
				msgs = append(msgs, irMessage{Role: "system", Parts: []irPart{textPart(irText(parts))}})
			} else {
				msgs = append(msgs, irMessage{Role: role, Parts: parts})
			}
		case "function_call", "custom_tool_call":
			args := rawString(it["arguments"])
			if args == "" {
				args = "{}"
			}
			msgs = append(msgs, irMessage{
				Role: "assistant",
				Parts: []irPart{{
					Kind: irKindToolCall,
					ID:   firstNonEmpty(rawString(it["call_id"]), rawString(it["id"])),
					Name: rawString(it["name"]),
					Args: args,
				}},
			})
		case "function_call_output", "custom_tool_call_output":
			text := rawString(it["output"]) + irText(responsesContentToIR(it["content"]))
			id := firstNonEmpty(rawString(it["call_id"]), rawString(it["id"]))
			msgs = append(msgs, irMessage{
				Role:       "tool",
				ToolCallID: id,
				Parts:      []irPart{{Kind: irKindToolResult, ToolUseID: id, Nested: []irPart{textPart(text)}}},
			})
		case "reasoning":
			text := responsesOutputText(it["summary"]) + responsesOutputText(it["content"])
			sig := rawString(it["encrypted_content"])
			if text != "" || sig != "" {
				msgs = append(msgs, irMessage{Role: "assistant", Parts: []irPart{{Kind: irKindThinking, Text: text, Signature: sig}}})
			}
		}
	}
	return msgs
}

func responsesContentToIR(raw json.RawMessage) []irPart {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if s == "" {
			return nil
		}
		return []irPart{textPart(s)}
	}
	var parts []map[string]any
	if json.Unmarshal(raw, &parts) != nil {
		return []irPart{textPart(string(raw))}
	}
	var out []irPart
	for _, p := range parts {
		typ, _ := p["type"].(string)
		switch typ {
		case "input_text", "output_text", "text":
			t, _ := p["text"].(string)
			out = append(out, textPart(t))
		case "input_image", "image":
			url := ""
			if iu, ok := p["image_url"].(string); ok {
				url = iu
			} else if iu, ok := p["image_url"].(map[string]any); ok {
				url, _ = iu["url"].(string)
			}
			if url != "" {
				out = append(out, imageURLToIR(url))
			}
		case "input_file", "file":
			data, _ := p["file_data"].(string)
			if data == "" {
				if f, ok := p["file"].(map[string]any); ok {
					data, _ = f["file_data"].(string)
				}
			}
			name, _ := p["filename"].(string)
			if data == "" {
				continue
			}
			mime, raw := parseDataURL(data)
			if raw == "" {
				raw = data
				mime = "application/octet-stream"
			}
			out = append(out, mediaPart(irKindDocument, mime, raw, "", name))
		}
	}
	return out
}

func irToResponsesRequest(ir irRequest) ([]byte, error) {
	out := map[string]any{"stream": ir.Stream}
	if ir.Model != "" {
		out["model"] = ir.Model
	}
	if ir.MaxTokens > 0 {
		out["max_output_tokens"] = ir.MaxTokens
	}
	if ir.Temperature != nil {
		out["temperature"] = *ir.Temperature
	}
	if ir.TopP != nil {
		out["top_p"] = *ir.TopP
	}
	if ir.ReasoningEffort != "" {
		out["reasoning"] = map[string]string{"effort": ir.ReasoningEffort}
	}
	var instructions []string
	var input []any
	for _, msg := range ir.Messages {
		switch msg.Role {
		case "system":
			instructions = append(instructions, irText(msg.Parts))
		case "tool":
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": firstNonEmpty(msg.ToolCallID, nestedToolID(msg.Parts)),
				"output":  irText(msg.Parts),
			})
		default:
			rest, calls := splitIRToolCalls(msg.Parts)
			if think := irThinking(rest); think != "" || irThinkingSignature(rest) != "" {
				item := map[string]any{
					"type":    "reasoning",
					"summary": []any{map[string]any{"type": "summary_text", "text": think}},
				}
				if sig := irThinkingSignature(rest); sig != "" {
					item["encrypted_content"] = sig
				}
				input = append(input, item)
			}
			if len(calls) > 0 {
				for _, tc := range calls {
					input = append(input, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      tc.Name,
						"arguments": tc.Args,
					})
				}
				continue
			}
			role := msg.Role
			if role == "" {
				role = "user"
			}
			input = append(input, map[string]any{
				"type":    "message",
				"role":    role,
				"content": irPartsToResponsesContent(rest, role),
			})
		}
	}
	if len(instructions) > 0 {
		out["instructions"] = strings.Join(instructions, "\n")
	}
	out["input"] = input
	var tools []any
	for _, t := range ir.Tools {
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters":  json.RawMessage(t.Parameters),
		})
	}
	if len(tools) > 0 {
		out["tools"] = tools
	}
	if ir.ToolChoice != nil {
		out["tool_choice"] = ir.ToolChoice
	}
	return json.Marshal(out)
}

func irPartsToResponsesContent(parts []irPart, role string) any {
	textType := "input_text"
	imgType := "input_image"
	if role == "assistant" {
		textType = "output_text"
	}
	var out []map[string]any
	for _, p := range parts {
		switch p.Kind {
		case irKindText:
			out = append(out, map[string]any{"type": textType, "text": p.Text})
		case irKindImage:
			url := p.URL
			if p.Data != "" {
				mime := p.MIME
				if mime == "" {
					mime = "image/png"
				}
				url = encodeDataURL(mime, p.Data)
			}
			if url != "" {
				out = append(out, map[string]any{"type": imgType, "image_url": url})
			}
		case irKindDocument:
			if p.Data == "" {
				continue
			}
			fn := p.Filename
			if fn == "" {
				fn = filenameFromMIME(p.MIME)
			}
			mime := p.MIME
			if mime == "" {
				mime = "application/octet-stream"
			}
			out = append(out, map[string]any{
				"type":      "input_file",
				"filename":  fn,
				"file_data": encodeDataURL(mime, p.Data),
			})
		}
	}
	if len(out) == 0 {
		return []map[string]any{{"type": textType, "text": ""}}
	}
	return out
}

func responsesToIRResponse(body []byte) (irResponse, error) {
	var in responsesResp
	if err := json.Unmarshal(body, &in); err != nil {
		return irResponse{}, fmt.Errorf("Responses 响应不是合法 JSON: %w", err)
	}
	if in.Error != nil && in.Error.Message != "" {
		return irResponse{ErrorMessage: in.Error.Message, ErrorType: in.Error.Type}, nil
	}
	out := irResponse{ID: in.ID, Model: in.Model, FinishReason: "stop"}
	if in.Usage != nil {
		out.PromptTokens = in.Usage.InputTokens
		out.CompletionTokens = in.Usage.OutputTokens
		if in.Usage.InputTokensDetails != nil {
			out.CacheReadTokens = in.Usage.InputTokensDetails.CachedTokens
		}
		if in.Usage.OutputTokensDetails != nil {
			out.ReasoningTokens = in.Usage.OutputTokensDetails.ReasoningTokens
		}
	}
	for _, item := range in.Output {
		switch item.Type {
		case "message":
			out.Parts = append(out.Parts, responsesContentToIR(item.Content)...)
		case "function_call", "custom_tool_call":
			args := item.Args
			if args == "" {
				args = "{}"
			}
			out.Parts = append(out.Parts, irPart{
				Kind: irKindToolCall,
				ID:   firstNonEmpty(item.CallID, item.ID),
				Name: item.Name,
				Args: args,
			})
			out.FinishReason = "tool_calls"
		case "reasoning":
			t := responsesOutputText(item.Summary) + responsesOutputText(item.Content)
			if t != "" || item.EncryptedContent != "" {
				out.Parts = append(out.Parts, irPart{Kind: irKindThinking, Text: t, Signature: item.EncryptedContent})
			}
		}
	}
	return out, nil
}

func irToResponsesResponse(ir irResponse) ([]byte, error) {
	if ir.ErrorMessage != "" {
		return json.Marshal(map[string]any{"error": map[string]any{"message": ir.ErrorMessage, "type": ir.ErrorType}})
	}
	id := ir.ID
	if id == "" {
		id = "resp_caosi"
	}
	out := map[string]any{"id": id, "object": "response", "status": "completed", "model": ir.Model}
	var output []any
	if think := irThinking(ir.Parts); think != "" || irThinkingSignature(ir.Parts) != "" {
		item := map[string]any{
			"type":    "reasoning",
			"summary": []map[string]any{{"type": "summary_text", "text": think}},
		}
		if sig := irThinkingSignature(ir.Parts); sig != "" {
			item["encrypted_content"] = sig
		}
		output = append(output, item)
	}
	if text := irText(ir.Parts); text != "" {
		output = append(output, map[string]any{
			"type":    "message",
			"role":    "assistant",
			"content": []map[string]any{{"type": "output_text", "text": text}},
		})
	}
	for _, p := range ir.Parts {
		if p.Kind == irKindToolCall {
			output = append(output, map[string]any{
				"type":      "function_call",
				"call_id":   p.ID,
				"name":      p.Name,
				"arguments": p.Args,
			})
		}
	}
	out["output"] = output
	if ir.PromptTokens != 0 || ir.CompletionTokens != 0 || ir.ReasoningTokens != 0 || ir.CacheReadTokens != 0 {
		usage := map[string]any{"input_tokens": ir.PromptTokens, "output_tokens": ir.CompletionTokens}
		if ir.CacheReadTokens != 0 {
			usage["input_tokens_details"] = map[string]int{"cached_tokens": ir.CacheReadTokens}
		}
		if ir.ReasoningTokens != 0 {
			usage["output_tokens_details"] = map[string]int{"reasoning_tokens": ir.ReasoningTokens}
		}
		out["usage"] = usage
	}
	return json.Marshal(out)
}
