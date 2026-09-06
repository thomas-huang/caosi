package convert

import (
	"encoding/json"
	"fmt"
	"strings"
)

func claudeToIRRequest(body []byte, model string, stream bool) (irRequest, error) {
	var in claudeReq
	if err := json.Unmarshal(body, &in); err != nil {
		return irRequest{}, fmt.Errorf("Claude 请求不是合法 JSON: %w", err)
	}
	out := irRequest{
		Stream:       stream,
		MaxTokens:    in.MaxTokens,
		Temperature:  in.Temperature,
		TopP:         in.TopP,
		Stop:         in.StopSequences,
		IncludeUsage: stream,
	}
	if model != "" {
		out.Model = model
	} else {
		out.Model = in.Model
	}
	if in.Thinking != nil {
		switch in.Thinking.Type {
		case "enabled", "adaptive", "auto":
			out.ReasoningEffort = "medium"
			if in.Thinking.BudgetTokens >= 16000 {
				out.ReasoningEffort = "high"
			} else if in.Thinking.BudgetTokens > 0 && in.Thinking.BudgetTokens < 2000 {
				out.ReasoningEffort = "low"
			}
		case "disabled":
			out.ReasoningEffort = "low"
		}
	}
	if sys := systemText(in.System); sys != "" {
		out.Messages = append(out.Messages, irMessage{Role: "system", Parts: []irPart{textPart(sys)}})
	}
	for _, msg := range in.Messages {
		out.Messages = append(out.Messages, claudeMessageToIR(msg)...)
	}
	for _, t := range in.Tools {
		params := t.InputSchema
		if len(params) == 0 {
			params = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out.Tools = append(out.Tools, irTool{Name: t.Name, Description: t.Description, Parameters: params})
	}
	if tc := convertToolChoice(in.ToolChoice); tc != nil {
		out.ToolChoice = tc
	}
	return out, nil
}

func claudeMessageToIR(msg claudeMsg) []irMessage {
	role := msg.Role
	if role == "" {
		role = "user"
	}
	var asString string
	if err := json.Unmarshal(msg.Content, &asString); err == nil {
		return []irMessage{{Role: role, Parts: []irPart{textPart(asString)}}}
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return []irMessage{{Role: role}}
	}
	var main irMessage
	main.Role = role
	var extra []irMessage
	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			main.Parts = append(main.Parts, textPart(bl.Text))
		case "image":
			if p, ok := claudeSourceToIR(irKindImage, bl.Source); ok {
				main.Parts = append(main.Parts, p)
			}
		case "document":
			if p, ok := claudeSourceToIR(irKindDocument, bl.Source); ok {
				main.Parts = append(main.Parts, p)
			}
		case "tool_use":
			args := "{}"
			if len(bl.Input) > 0 {
				args = string(bl.Input)
			}
			main.Parts = append(main.Parts, irPart{Kind: irKindToolCall, ID: bl.ID, Name: bl.Name, Args: args})
		case "tool_result":
			extra = append(extra, irMessage{
				Role:       "tool",
				ToolCallID: bl.ToolUseID,
				Parts:      []irPart{{Kind: irKindToolResult, ToolUseID: bl.ToolUseID, Nested: claudeToolResultParts(bl.Content)}},
			})
		case "thinking":
			t := bl.Thinking
			if t == "" {
				t = bl.Text
			}
			if t != "" {
				main.Parts = append(main.Parts, thinkingPart(t))
			}
		}
	}
	if len(main.Parts) == 0 && len(extra) > 0 {
		return extra
	}
	if len(main.Parts) == 0 {
		return []irMessage{{Role: role}}
	}
	return append([]irMessage{main}, extra...)
}

func claudeSourceToIR(kind irKind, src *claudeImgSrc) (irPart, bool) {
	if src == nil {
		return irPart{}, false
	}
	if src.Type == "file" {
		return irPart{}, false
	}
	if src.Type == "url" || src.URL != "" && src.Data == "" {
		if src.URL == "" {
			return irPart{}, false
		}
		return mediaPart(kind, src.MediaType, "", src.URL, ""), true
	}
	if src.Data == "" {
		return irPart{}, false
	}
	mime := src.MediaType
	if mime == "" && kind == irKindImage {
		mime = "image/png"
	}
	return mediaPart(kind, mime, src.Data, "", ""), true
}

func claudeToolResultParts(raw json.RawMessage) []irPart {
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
	var blocks []claudeBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return []irPart{textPart(string(raw))}
	}
	var parts []irPart
	for _, bl := range blocks {
		switch bl.Type {
		case "text", "":
			if bl.Text != "" {
				parts = append(parts, textPart(bl.Text))
			}
		case "image":
			if p, ok := claudeSourceToIR(irKindImage, bl.Source); ok {
				parts = append(parts, p)
			}
		case "document":
			if p, ok := claudeSourceToIR(irKindDocument, bl.Source); ok {
				parts = append(parts, p)
			}
		}
	}
	return parts
}

func irToClaudeRequest(ir irRequest) ([]byte, error) {
	out := map[string]any{"stream": ir.Stream, "max_tokens": 4096}
	if ir.Model != "" {
		out["model"] = ir.Model
	}
	if ir.MaxTokens > 0 {
		out["max_tokens"] = ir.MaxTokens
	}
	if ir.Temperature != nil {
		out["temperature"] = *ir.Temperature
	}
	if ir.TopP != nil {
		out["top_p"] = *ir.TopP
	}
	if len(ir.Stop) > 0 {
		out["stop_sequences"] = ir.Stop
	}
	if ir.ReasoningEffort != "" && ir.ReasoningEffort != "none" {
		out["thinking"] = map[string]any{"type": "enabled", "budget_tokens": 4096}
	}
	var msgs []any
	for _, m := range ir.Messages {
		switch m.Role {
		case "system":
			out["system"] = irText(m.Parts)
		case "tool":
			msgs = append(msgs, map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type":        "tool_result",
					"tool_use_id": firstNonEmpty(m.ToolCallID, nestedToolID(m.Parts)),
					"content":     irPartsToClaudeToolResult(m.Parts),
				}},
			})
		default:
			role := m.Role
			if role == "" {
				role = "user"
			}
			blocks := irPartsToClaudeBlocks(m.Parts)
			if len(blocks) == 1 {
				if b, ok := blocks[0].(map[string]any); ok && b["type"] == "text" {
					msgs = append(msgs, map[string]any{"role": role, "content": b["text"]})
					continue
				}
			}
			if len(blocks) == 0 {
				msgs = append(msgs, map[string]any{"role": role, "content": ""})
				continue
			}
			msgs = append(msgs, map[string]any{"role": role, "content": blocks})
		}
	}
	out["messages"] = msgs
	var tools []any
	for _, t := range ir.Tools {
		tools = append(tools, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": json.RawMessage(t.Parameters),
		})
	}
	if len(tools) > 0 {
		out["tools"] = tools
	}
	return json.Marshal(out)
}

func irPartsToClaudeBlocks(parts []irPart) []any {
	var blocks []any
	for _, p := range parts {
		switch p.Kind {
		case irKindThinking:
			if p.Text != "" {
				blocks = append(blocks, map[string]any{"type": "thinking", "thinking": p.Text})
			}
		case irKindText:
			if p.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
			}
		case irKindImage:
			if b, ok := irMediaToClaudeBlock("image", p); ok {
				blocks = append(blocks, b)
			}
		case irKindDocument:
			if b, ok := irMediaToClaudeBlock("document", p); ok {
				blocks = append(blocks, b)
			}
		case irKindToolCall:
			var input any = json.RawMessage([]byte("{}"))
			if json.Valid([]byte(p.Args)) {
				input = json.RawMessage(p.Args)
			}
			blocks = append(blocks, map[string]any{"type": "tool_use", "id": p.ID, "name": p.Name, "input": input})
		}
	}
	return blocks
}

func irMediaToClaudeBlock(typ string, p irPart) (map[string]any, bool) {
	if p.Data != "" {
		mime := p.MIME
		if mime == "" && typ == "image" {
			mime = "image/png"
		}
		if mime == "" {
			mime = "application/octet-stream"
		}
		return map[string]any{
			"type":   typ,
			"source": map[string]any{"type": "base64", "media_type": mime, "data": p.Data},
		}, true
	}
	if p.URL != "" {
		return map[string]any{
			"type":   typ,
			"source": map[string]any{"type": "url", "url": p.URL},
		}, true
	}
	return nil, false
}

func irPartsToClaudeToolResult(parts []irPart) any {
	var nested []irPart
	for _, p := range parts {
		if p.Kind == irKindToolResult {
			nested = append(nested, p.Nested...)
			continue
		}
		nested = append(nested, p)
	}
	var blocks []any
	onlyText := true
	var text strings.Builder
	for _, p := range nested {
		switch p.Kind {
		case irKindText:
			text.WriteString(p.Text)
			if p.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
			}
		case irKindImage:
			if b, ok := irMediaToClaudeBlock("image", p); ok {
				blocks = append(blocks, b)
				onlyText = false
			}
		case irKindDocument:
			if b, ok := irMediaToClaudeBlock("document", p); ok {
				blocks = append(blocks, b)
				onlyText = false
			}
		}
	}
	if onlyText {
		return text.String()
	}
	return blocks
}

func claudeToIRResponse(body []byte) (irResponse, error) {
	var in claudeResp
	if err := json.Unmarshal(body, &in); err != nil {
		return irResponse{}, fmt.Errorf("Claude 响应不是合法 JSON: %w", err)
	}
	out := irResponse{ID: in.ID, Model: in.Model, PromptTokens: in.Usage.InputTokens, CompletionTokens: in.Usage.OutputTokens}
	switch in.StopReason {
	case "tool_use":
		out.FinishReason = "tool_calls"
	case "max_tokens":
		out.FinishReason = "length"
	default:
		out.FinishReason = "stop"
	}
	for _, b := range in.Content {
		switch b.Type {
		case "text":
			out.Parts = append(out.Parts, textPart(b.Text))
		case "thinking":
			t := b.Thinking
			if t == "" {
				t = b.Text
			}
			out.Parts = append(out.Parts, thinkingPart(t))
		case "tool_use":
			args := string(b.Input)
			if args == "" {
				args = "{}"
			}
			out.Parts = append(out.Parts, irPart{Kind: irKindToolCall, ID: b.ID, Name: b.Name, Args: args})
			out.FinishReason = "tool_calls"
		}
	}
	return out, nil
}

func irToClaudeResponse(ir irResponse) ([]byte, error) {
	if ir.ErrorMessage != "" {
		return encodeClaudeError(ir.ErrorType, ir.ErrorMessage)
	}
	out := claudeResp{ID: ir.ID, Type: "message", Role: "assistant", Model: ir.Model}
	if out.ID == "" {
		out.ID = "msg_caosi"
	}
	out.Usage.InputTokens = ir.PromptTokens
	out.Usage.OutputTokens = ir.CompletionTokens
	out.StopReason = mapFinishReason(ir.FinishReason)
	for _, p := range ir.Parts {
		switch p.Kind {
		case irKindThinking:
			out.Content = append(out.Content, claudeBlock{Type: "thinking", Thinking: p.Text})
		case irKindText:
			if p.Text != "" {
				out.Content = append(out.Content, claudeBlock{Type: "text", Text: p.Text})
			}
		case irKindToolCall:
			var input json.RawMessage
			if json.Valid([]byte(p.Args)) {
				input = json.RawMessage(p.Args)
			} else {
				b, _ := json.Marshal(p.Args)
				input = b
			}
			out.Content = append(out.Content, claudeBlock{Type: "tool_use", ID: p.ID, Name: p.Name, Input: input})
			if out.StopReason == "end_turn" {
				out.StopReason = "tool_use"
			}
		}
	}
	if len(out.Content) == 0 {
		out.Content = []claudeBlock{}
	}
	return json.Marshal(out)
}
