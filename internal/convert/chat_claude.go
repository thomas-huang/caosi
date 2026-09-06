package convert

import (
	"encoding/json"
	"fmt"
)

func openAIChatToClaudeRequest(body []byte, model string, stream bool) ([]byte, error) {
	var in openaiChatReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Chat 请求不是合法 JSON: %w", err)
	}
	out := map[string]any{"stream": stream, "max_tokens": 4096}
	if model != "" {
		out["model"] = model
	} else {
		out["model"] = in.Model
	}
	if in.MaxTokens > 0 {
		out["max_tokens"] = in.MaxTokens
	}
	if in.Temperature != nil {
		out["temperature"] = *in.Temperature
	}
	if in.TopP != nil {
		out["top_p"] = *in.TopP
	}
	if len(in.Stop) > 0 {
		out["stop_sequences"] = in.Stop
	}
	if in.ReasoningEffort != "" && in.ReasoningEffort != "none" {
		out["thinking"] = map[string]any{"type": "enabled", "budget_tokens": 4096}
	}
	var msgs []any
	for _, m := range in.Messages {
		switch m.Role {
		case "system":
			out["system"] = contentAsString(m.Content)
		case "tool":
			msgs = append(msgs, map[string]any{
				"role": "user",
				"content": []any{map[string]any{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     contentAsString(m.Content),
				}},
			})
		default:
			role := m.Role
			if role == "" {
				role = "user"
			}
			var blocks []any
			if m.ReasoningContent != "" {
				blocks = append(blocks, map[string]any{"type": "thinking", "thinking": m.ReasoningContent})
			}
			if s := contentAsString(m.Content); s != "" && !hasImageContent(m.Content) {
				blocks = append(blocks, map[string]any{"type": "text", "text": s})
			} else {
				blocks = append(blocks, chatContentToClaudeBlocks(m.Content)...)
			}
			for _, tc := range m.ToolCalls {
				var input any = json.RawMessage([]byte("{}"))
				if json.Valid([]byte(tc.Function.Arguments)) {
					input = json.RawMessage(tc.Function.Arguments)
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": input,
				})
			}
			if len(blocks) == 1 {
				if b, ok := blocks[0].(map[string]any); ok && b["type"] == "text" {
					msgs = append(msgs, map[string]any{"role": role, "content": b["text"]})
					continue
				}
			}
			msgs = append(msgs, map[string]any{"role": role, "content": blocks})
		}
	}
	out["messages"] = msgs
	var tools []any
	for _, t := range in.Tools {
		tools = append(tools, map[string]any{
			"name":         t.Function.Name,
			"description":  t.Function.Description,
			"input_schema": json.RawMessage(t.Function.Parameters),
		})
	}
	if len(tools) > 0 {
		out["tools"] = tools
	}
	return json.Marshal(out)
}

func hasImageContent(content any) bool {
	b, _ := json.Marshal(content)
	var parts []openaiPart
	if json.Unmarshal(b, &parts) != nil {
		return false
	}
	for _, p := range parts {
		if p.Type == "image_url" {
			return true
		}
	}
	return false
}

func chatContentToClaudeBlocks(content any) []any {
	b, _ := json.Marshal(content)
	var parts []openaiPart
	if json.Unmarshal(b, &parts) != nil {
		s := contentAsString(content)
		if s == "" {
			return nil
		}
		return []any{map[string]any{"type": "text", "text": s}}
	}
	var blocks []any
	for _, p := range parts {
		if p.Type == "image_url" && p.ImageURL != nil {
			url := p.ImageURL.URL
			mt, data := splitDataURL(url)
			if data != "" {
				blocks = append(blocks, map[string]any{
					"type":   "image",
					"source": map[string]any{"type": "base64", "media_type": mt, "data": data},
				})
			} else {
				blocks = append(blocks, map[string]any{
					"type":   "image",
					"source": map[string]any{"type": "url", "url": url},
				})
			}
		} else if p.Text != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
		}
	}
	return blocks
}

func claudeToChatResponse(body []byte) ([]byte, error) {
	var in claudeResp
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Claude 响应不是合法 JSON: %w", err)
	}
	content := ""
	thinking := ""
	var tcs []openaiToolCall
	finish := "stop"
	if in.StopReason == "tool_use" {
		finish = "tool_calls"
	}
	if in.StopReason == "max_tokens" {
		finish = "length"
	}
	for _, b := range in.Content {
		switch b.Type {
		case "text":
			content += b.Text
		case "thinking":
			thinking += b.Thinking
			if thinking == "" {
				thinking = b.Text
			}
		case "tool_use":
			tc := openaiToolCall{ID: b.ID, Type: "function"}
			tc.Function.Name = b.Name
			tc.Function.Arguments = string(b.Input)
			if tc.Function.Arguments == "" {
				tc.Function.Arguments = "{}"
			}
			tcs = append(tcs, tc)
			finish = "tool_calls"
		}
	}
	m := map[string]any{
		"id":    in.ID,
		"model": in.Model,
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
		"usage": map[string]int{
			"prompt_tokens":     in.Usage.InputTokens,
			"completion_tokens": in.Usage.OutputTokens,
		},
	}
	return json.Marshal(m)
}
