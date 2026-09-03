package convert

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thomas-huang/caosi/internal/config"
)

type claudeReq struct {
	Model         string          `json:"model"`
	MaxTokens     int             `json:"max_tokens"`
	Temperature   *float64        `json:"temperature"`
	TopP          *float64        `json:"top_p"`
	Stream        bool            `json:"stream"`
	StopSequences []string        `json:"stop_sequences"`
	System        json.RawMessage `json:"system"`
	Messages      []claudeMsg     `json:"messages"`
	Tools         []claudeTool    `json:"tools"`
	ToolChoice    json.RawMessage `json:"tool_choice"`
	Thinking      *claudeThinking `json:"thinking"`
}

type claudeThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

type claudeMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type claudeBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
	Source    *claudeImgSrc   `json:"source,omitempty"`
}

type claudeImgSrc struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
	URL       string `json:"url"`
}

type openaiChatReq struct {
	Model           string       `json:"model"`
	Messages        []openaiMsg  `json:"messages"`
	MaxTokens       int          `json:"max_tokens,omitempty"`
	Temperature     *float64     `json:"temperature,omitempty"`
	TopP            *float64     `json:"top_p,omitempty"`
	Stream          bool         `json:"stream,omitempty"`
	Stop            []string     `json:"stop,omitempty"`
	Tools           []openaiTool `json:"tools,omitempty"`
	ToolChoice      any          `json:"tool_choice,omitempty"`
	ReasoningEffort string       `json:"reasoning_effort,omitempty"`
	StreamOptions   *streamOpts  `json:"stream_options,omitempty"`
}

type streamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiMsg struct {
	Role             string           `json:"role"`
	Content          any              `json:"content,omitempty"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []openaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string           `json:"tool_call_id,omitempty"`
	Name             string           `json:"name,omitempty"`
}

type openaiTool struct {
	Type     string       `json:"type"`
	Function openaiToolFn `json:"function"`
}

type openaiToolFn struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openaiPart struct {
	Type     string        `json:"type"`
	Text     string        `json:"text,omitempty"`
	ImageURL *openaiImgURL `json:"image_url,omitempty"`
}

type openaiImgURL struct {
	URL string `json:"url"`
}

func ClaudeToOpenAIChat(body []byte, model string, stream bool) ([]byte, error) {
	var in claudeReq
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("Claude 请求不是合法 JSON: %w", err)
	}
	out := openaiChatReq{Stream: stream}
	if model != "" {
		out.Model = model
	} else {
		out.Model = in.Model
	}
	if in.MaxTokens > 0 {
		out.MaxTokens = in.MaxTokens
	}
	out.Temperature = in.Temperature
	out.TopP = in.TopP
	if len(in.StopSequences) > 0 {
		out.Stop = in.StopSequences
	}
	if stream {
		out.StreamOptions = &streamOpts{IncludeUsage: true}
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
		out.Messages = append(out.Messages, openaiMsg{Role: "system", Content: sys})
	}

	for _, msg := range in.Messages {
		converted, err := claudeMessageToOpenAI(msg)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, converted...)
	}

	for _, t := range in.Tools {
		params := t.InputSchema
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
	if tc := convertToolChoice(in.ToolChoice); tc != nil {
		out.ToolChoice = tc
	}

	return json.Marshal(out)
}

func systemText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, bl := range blocks {
			if bl.Type == "text" || bl.Type == "" {
				b.WriteString(bl.Text)
			}
		}
		return b.String()
	}
	return ""
}

func claudeMessageToOpenAI(msg claudeMsg) ([]openaiMsg, error) {
	role := msg.Role
	if role == "" {
		role = "user"
	}
	var asString string
	if err := json.Unmarshal(msg.Content, &asString); err == nil {
		return []openaiMsg{{Role: role, Content: asString}}, nil
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return []openaiMsg{{Role: role, Content: ""}}, nil
	}

	var (
		parts     []openaiPart
		toolCalls []openaiToolCall
		out       []openaiMsg
		textBuf   strings.Builder
	)

	for _, bl := range blocks {
		switch bl.Type {
		case "text":
			if looksLikeImageOnly(blocks) {
				parts = append(parts, openaiPart{Type: "text", Text: bl.Text})
			} else {
				textBuf.WriteString(bl.Text)
			}
		case "image":
			url := ""
			if bl.Source != nil {
				if bl.Source.Type == "url" {
					url = bl.Source.URL
				} else if bl.Source.Data != "" {
					mt := bl.Source.MediaType
					if mt == "" {
						mt = "image/png"
					}
					url = "data:" + mt + ";base64," + bl.Source.Data
				}
			}
			if url != "" {
				parts = append(parts, openaiPart{Type: "image_url", ImageURL: &openaiImgURL{URL: url}})
			}
		case "tool_use":
			args := "{}"
			if len(bl.Input) > 0 {
				args = string(bl.Input)
			}
			tc := openaiToolCall{ID: bl.ID, Type: "function"}
			tc.Function.Name = bl.Name
			tc.Function.Arguments = args
			toolCalls = append(toolCalls, tc)
		case "tool_result":
			content := toolResultText(bl.Content)
			out = append(out, openaiMsg{
				Role:       "tool",
				Content:    content,
				ToolCallID: bl.ToolUseID,
			})
		case "thinking":
			// analogue is reasoning_effort on the request, not a message block
		}
	}

	if len(toolCalls) > 0 {
		content := any(nil)
		if textBuf.Len() > 0 {
			content = textBuf.String()
		}
		out = append([]openaiMsg{{Role: "assistant", Content: content, ToolCalls: toolCalls}}, out...)
		return out, nil
	}
	if len(out) > 0 && textBuf.Len() == 0 && len(parts) == 0 {
		return out, nil
	}
	if len(parts) > 0 {
		if textBuf.Len() > 0 {
			parts = append([]openaiPart{{Type: "text", Text: textBuf.String()}}, parts...)
		}
		return append([]openaiMsg{{Role: role, Content: parts}}, out...), nil
	}
	if textBuf.Len() > 0 || len(out) == 0 {
		return append([]openaiMsg{{Role: role, Content: textBuf.String()}}, out...), nil
	}
	return out, nil
}

func looksLikeImageOnly(blocks []claudeBlock) bool {
	for _, bl := range blocks {
		if bl.Type == "image" {
			return true
		}
	}
	return false
}

func toolResultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, bl := range blocks {
			b.WriteString(bl.Text)
		}
		return b.String()
	}
	return string(raw)
}

func convertToolChoice(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		switch s {
		case "auto", "none":
			return s
		case "any", "required":
			return "required"
		}
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		if t, _ := obj["type"].(string); t == "tool" {
			if name, _ := obj["name"].(string); name != "" {
				return map[string]any{
					"type":     "function",
					"function": map[string]string{"name": name},
				}
			}
		}
	}
	return nil
}

type openaiChatResp struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
	Choices []struct {
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Role             string           `json:"role"`
			Content          string           `json:"content"`
			ReasoningContent string           `json:"reasoning_content"`
			ToolCalls        []openaiToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

type claudeResp struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Role       string        `json:"role"`
	Model      string        `json:"model"`
	Content    []claudeBlock `json:"content"`
	StopReason string        `json:"stop_reason"`
	Usage      claudeUsage   `json:"usage"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func OpenAIChatToClaude(body []byte) ([]byte, error) {
	var in openaiChatResp
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("OpenAI 响应不是合法 JSON: %w", err)
	}
	if in.Error != nil && in.Error.Message != "" {
		return ClaudeError(in.Error.Type, in.Error.Message)
	}
	out := claudeResp{
		ID:    in.ID,
		Type:  "message",
		Role:  "assistant",
		Model: in.Model,
	}
	if out.ID == "" {
		out.ID = "msg_caosi"
	}
	if in.Usage != nil {
		out.Usage.InputTokens = in.Usage.PromptTokens
		out.Usage.OutputTokens = in.Usage.CompletionTokens
	}
	if len(in.Choices) == 0 {
		out.Content = []claudeBlock{}
		out.StopReason = "end_turn"
		return json.Marshal(out)
	}
	ch := in.Choices[0]
	out.StopReason = mapFinishReason(ch.FinishReason)
	if len(ch.Message.ToolCalls) > 0 && out.StopReason == "end_turn" {
		out.StopReason = "tool_use"
	}
	if ch.Message.ReasoningContent != "" {
		out.Content = append(out.Content, claudeBlock{Type: "thinking", Thinking: ch.Message.ReasoningContent})
	}
	if ch.Message.Content != "" {
		out.Content = append(out.Content, claudeBlock{Type: "text", Text: ch.Message.Content})
	}
	for _, tc := range ch.Message.ToolCalls {
		var input json.RawMessage
		if json.Valid([]byte(tc.Function.Arguments)) {
			input = json.RawMessage(tc.Function.Arguments)
		} else {
			b, _ := json.Marshal(tc.Function.Arguments)
			input = b
		}
		out.Content = append(out.Content, claudeBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	if len(out.Content) == 0 {
		out.Content = []claudeBlock{}
	}
	return json.Marshal(out)
}

func mapFinishReason(r string) string {
	switch r {
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}

func ClaudeError(typ, msg string) ([]byte, error) {
	if typ == "" {
		typ = "api_error"
	}
	var e claudeError
	e.Type = "error"
	e.Error.Type = typ
	e.Error.Message = msg
	return json.Marshal(e)
}

func OpenAIError(msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "invalid_request_error",
		},
	})
	return b
}

func GeminiError(msg string, code int) []byte {
	status := "INVALID_ARGUMENT"
	if code >= 500 {
		status = "INTERNAL"
	}
	b, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
			"status":  status,
		},
	})
	return b
}

func ClientError(client config.Protocol, status int, msg string) (int, []byte, string) {
	switch client {
	case config.ProtocolClaudeMessages:
		b, _ := ClaudeError("invalid_request_error", msg)
		return status, b, "application/json"
	case config.ProtocolGemini:
		if status < 400 {
			status = 400
		}
		return status, GeminiError(msg, status), "application/json"
	default:
		return status, OpenAIError(msg), "application/json"
	}
}
