package convert

import (
	"encoding/json"
	"fmt"
	"strings"
)

type responsesReq struct {
	Model           string          `json:"model"`
	Input           json.RawMessage `json:"input"`
	Instructions    string          `json:"instructions,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Stream          bool            `json:"stream,omitempty"`
	Tools           []responsesTool `json:"tools,omitempty"`
	ToolChoice      any             `json:"tool_choice,omitempty"`
	Reasoning       *struct {
		Effort string `json:"effort,omitempty"`
	} `json:"reasoning,omitempty"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Function    *openaiToolFn   `json:"function,omitempty"`
}

type responsesResp struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	Status string `json:"status"`
	Model  string `json:"model"`
	Error  *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
	Output []responsesOut `json:"output"`
	Usage  *struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type responsesOut struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	CallID  string          `json:"call_id,omitempty"`
	Role    string          `json:"role,omitempty"`
	Name    string          `json:"name,omitempty"`
	Content json.RawMessage `json:"content,omitempty"`
	Args    string          `json:"arguments,omitempty"`
	Summary json.RawMessage `json:"summary,omitempty"`
}

func ResponsesToChat(body []byte, model string, stream bool) ([]byte, error) {
	var in responsesReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Responses 请求不是合法 JSON: %w", err)
	}
	out := openaiChatReq{Stream: stream}
	if model != "" {
		out.Model = model
	} else {
		out.Model = in.Model
	}
	if in.MaxOutputTokens > 0 {
		out.MaxTokens = in.MaxOutputTokens
	}
	out.Temperature = in.Temperature
	out.TopP = in.TopP
	if in.Reasoning != nil && in.Reasoning.Effort != "" {
		out.ReasoningEffort = in.Reasoning.Effort
	}
	if stream {
		out.StreamOptions = &streamOpts{IncludeUsage: true}
	}
	if in.Instructions != "" {
		out.Messages = append(out.Messages, openaiMsg{Role: "system", Content: in.Instructions})
	}
	out.Messages = append(out.Messages, responsesInputToMessages(in.Input)...)
	for _, t := range in.Tools {
		fn := openaiToolFn{Name: t.Name, Description: t.Description, Parameters: t.Parameters}
		if t.Function != nil {
			fn = *t.Function
		}
		if fn.Name == "" {
			continue
		}
		out.Tools = append(out.Tools, openaiTool{Type: "function", Function: fn})
	}
	if in.ToolChoice != nil {
		out.ToolChoice = in.ToolChoice
	}
	return json.Marshal(out)
}

func responsesInputToMessages(raw json.RawMessage) []openaiMsg {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		return []openaiMsg{{Role: "user", Content: asString}}
	}
	var items []map[string]json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil
	}
	var msgs []openaiMsg
	for _, it := range items {
		typ := rawString(it["type"])
		role := rawString(it["role"])
		switch typ {
		case "", "message", "input_text":
			if role == "" {
				role = "user"
			}
			content := responsesContentToChat(it["content"])
			if role == "system" {
				msgs = append(msgs, openaiMsg{Role: "system", Content: contentAsString(content)})
			} else {
				msgs = append(msgs, openaiMsg{Role: role, Content: content})
			}
		case "function_call", "custom_tool_call":
			tc := openaiToolCall{ID: firstNonEmpty(rawString(it["call_id"]), rawString(it["id"])), Type: "function"}
			tc.Function.Name = rawString(it["name"])
			tc.Function.Arguments = rawString(it["arguments"])
			if tc.Function.Arguments == "" {
				tc.Function.Arguments = "{}"
			}
			msgs = append(msgs, openaiMsg{Role: "assistant", ToolCalls: []openaiToolCall{tc}})
		case "function_call_output", "custom_tool_call_output":
			msgs = append(msgs, openaiMsg{
				Role:       "tool",
				ToolCallID: firstNonEmpty(rawString(it["call_id"]), rawString(it["id"])),
				Content:    rawString(it["output"]) + contentAsString(responsesContentToChat(it["content"])),
			})
		case "reasoning":
			// carried as reasoning_effort on the request; skip message
		}
	}
	return msgs
}

func responsesContentToChat(raw json.RawMessage) any {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []map[string]any
	if json.Unmarshal(raw, &parts) != nil {
		return string(raw)
	}
	var text strings.Builder
	var outParts []openaiPart
	for _, p := range parts {
		typ, _ := p["type"].(string)
		switch typ {
		case "input_text", "output_text", "text":
			t, _ := p["text"].(string)
			text.WriteString(t)
			outParts = append(outParts, openaiPart{Type: "text", Text: t})
		case "input_image", "image":
			url := ""
			if iu, ok := p["image_url"].(string); ok {
				url = iu
			} else if iu, ok := p["image_url"].(map[string]any); ok {
				url, _ = iu["url"].(string)
			}
			if url != "" {
				outParts = append(outParts, openaiPart{Type: "image_url", ImageURL: &openaiImgURL{URL: url}})
			}
		}
	}
	if len(outParts) == 1 && outParts[0].Type == "text" {
		return outParts[0].Text
	}
	if len(outParts) > 0 {
		return outParts
	}
	return text.String()
}

func ChatToResponses(body []byte, model string, stream bool) ([]byte, error) {
	var in openaiChatReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Chat 请求不是合法 JSON: %w", err)
	}
	out := map[string]any{"stream": stream}
	if model != "" {
		out["model"] = model
	} else {
		out["model"] = in.Model
	}
	if in.MaxTokens > 0 {
		out["max_output_tokens"] = in.MaxTokens
	}
	if in.Temperature != nil {
		out["temperature"] = *in.Temperature
	}
	if in.TopP != nil {
		out["top_p"] = *in.TopP
	}
	if in.ReasoningEffort != "" {
		out["reasoning"] = map[string]string{"effort": in.ReasoningEffort}
	}
	var instructions []string
	var input []any
	for _, msg := range in.Messages {
		switch msg.Role {
		case "system":
			instructions = append(instructions, contentAsString(msg.Content))
		case "tool":
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  contentAsString(msg.Content),
			})
		default:
			if msg.ReasoningContent != "" {
				input = append(input, map[string]any{
					"type":    "reasoning",
					"summary": []any{map[string]any{"type": "summary_text", "text": msg.ReasoningContent}},
				})
			}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					input = append(input, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      tc.Function.Name,
						"arguments": tc.Function.Arguments,
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
				"content": chatContentToResponses(msg.Content, role),
			})
		}
	}
	if len(instructions) > 0 {
		out["instructions"] = strings.Join(instructions, "\n")
	}
	out["input"] = input
	var tools []any
	for _, t := range in.Tools {
		tools = append(tools, map[string]any{
			"type":        "function",
			"name":        t.Function.Name,
			"description": t.Function.Description,
			"parameters":  json.RawMessage(t.Function.Parameters),
		})
	}
	if len(tools) > 0 {
		out["tools"] = tools
	}
	if in.ToolChoice != nil {
		out["tool_choice"] = in.ToolChoice
	}
	return json.Marshal(out)
}

func chatContentToResponses(content any, role string) any {
	textType := "input_text"
	imgType := "input_image"
	if role == "assistant" {
		textType = "output_text"
	}
	switch c := content.(type) {
	case string:
		return []map[string]any{{"type": textType, "text": c}}
	case []any:
		var parts []map[string]any
		for _, raw := range c {
			m, _ := raw.(map[string]any)
			if m == nil {
				continue
			}
			typ, _ := m["type"].(string)
			if typ == "image_url" {
				url := ""
				if iu, ok := m["image_url"].(map[string]any); ok {
					url, _ = iu["url"].(string)
				}
				parts = append(parts, map[string]any{"type": imgType, "image_url": url})
			} else {
				t, _ := m["text"].(string)
				parts = append(parts, map[string]any{"type": textType, "text": t})
			}
		}
		return parts
	default:
		b, _ := json.Marshal(content)
		var parts []openaiPart
		if json.Unmarshal(b, &parts) == nil && len(parts) > 0 {
			var out []map[string]any
			for _, p := range parts {
				if p.Type == "image_url" && p.ImageURL != nil {
					out = append(out, map[string]any{"type": imgType, "image_url": p.ImageURL.URL})
				} else {
					out = append(out, map[string]any{"type": textType, "text": p.Text})
				}
			}
			return out
		}
		return []map[string]any{{"type": textType, "text": contentAsString(content)}}
	}
}

func ResponsesToChatResponse(body []byte) ([]byte, error) {
	var in responsesResp
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Responses 响应不是合法 JSON: %w", err)
	}
	if in.Error != nil && in.Error.Message != "" {
		return json.Marshal(map[string]any{
			"error": map[string]any{"message": in.Error.Message, "type": in.Error.Type},
		})
	}
	content, thinking := "", ""
	var tcs []openaiToolCall
	finish := "stop"
	for _, item := range in.Output {
		switch item.Type {
		case "message":
			content += responsesOutputText(item.Content)
		case "function_call", "custom_tool_call":
			tc := openaiToolCall{ID: firstNonEmpty(item.CallID, item.ID), Type: "function"}
			tc.Function.Name = item.Name
			tc.Function.Arguments = item.Args
			if tc.Function.Arguments == "" {
				tc.Function.Arguments = "{}"
			}
			tcs = append(tcs, tc)
			finish = "tool_calls"
		case "reasoning":
			thinking += responsesOutputText(item.Summary) + responsesOutputText(item.Content)
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
	}
	if in.Usage != nil {
		m["usage"] = map[string]int{
			"prompt_tokens":     in.Usage.InputTokens,
			"completion_tokens": in.Usage.OutputTokens,
		}
	}
	return json.Marshal(m)
}

func ChatToResponsesResponse(body []byte) ([]byte, error) {
	var in openaiChatResp
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, err
	}
	id := in.ID
	if id == "" {
		id = "resp_caosi"
	}
	out := map[string]any{
		"id":     id,
		"object": "response",
		"status": "completed",
		"model":  in.Model,
	}
	var output []any
	if len(in.Choices) > 0 {
		ch := in.Choices[0]
		if ch.Message.ReasoningContent != "" {
			output = append(output, map[string]any{
				"type":    "reasoning",
				"summary": []map[string]any{{"type": "summary_text", "text": ch.Message.ReasoningContent}},
			})
		}
		if ch.Message.Content != "" {
			output = append(output, map[string]any{
				"type":    "message",
				"role":    "assistant",
				"content": []map[string]any{{"type": "output_text", "text": ch.Message.Content}},
			})
		}
		for _, tc := range ch.Message.ToolCalls {
			output = append(output, map[string]any{
				"type":      "function_call",
				"call_id":   tc.ID,
				"name":      tc.Function.Name,
				"arguments": tc.Function.Arguments,
			})
		}
	}
	out["output"] = output
	if in.Usage != nil {
		out["usage"] = map[string]int{
			"input_tokens":  in.Usage.PromptTokens,
			"output_tokens": in.Usage.CompletionTokens,
		}
	}
	return json.Marshal(out)
}

func ClaudeToResponses(body []byte, model string, stream bool) ([]byte, error) {
	chat, err := ClaudeToOpenAIChat(body, model, stream)
	if err != nil {
		return nil, err
	}
	return ChatToResponses(chat, model, stream)
}

func responsesOutputText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []map[string]any
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if t, ok := p["text"].(string); ok {
				b.WriteString(t)
			}
		}
		return b.String()
	}
	return ""
}

func rawString(r json.RawMessage) string {
	if len(r) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(r, &s) == nil {
		return s
	}
	return strings.Trim(string(r), `"`)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func contentAsString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		var parts []openaiPart
		if json.Unmarshal(b, &parts) == nil {
			var s strings.Builder
			for _, p := range parts {
				s.WriteString(p.Text)
			}
			return s.String()
		}
		return string(b)
	}
}
