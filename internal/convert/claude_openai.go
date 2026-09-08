package convert

import (
	"encoding/json"
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
	Signature string          `json:"signature,omitempty"`
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
	Audio            *openaiAudio     `json:"audio,omitempty"`
}

type openaiAudio struct {
	ID   string `json:"id,omitempty"`
	Data string `json:"data,omitempty"`
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
			Audio            *openaiAudio     `json:"audio"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		PromptTokensDetails     *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details,omitempty"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details,omitempty"`
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
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

type claudeError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
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

func encodeClaudeError(typ, msg string) ([]byte, error) {
	if typ == "" {
		typ = "api_error"
	}
	var e claudeError
	e.Type = "error"
	e.Error.Type = typ
	e.Error.Message = msg
	return json.Marshal(e)
}

func encodeOpenAIError(msg string) []byte {
	b, _ := json.Marshal(map[string]any{
		"error": map[string]any{
			"message": msg,
			"type":    "invalid_request_error",
		},
	})
	return b
}

func encodeGeminiError(msg string, code int) []byte {
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
		b, _ := encodeClaudeError("invalid_request_error", msg)
		return status, b, "application/json"
	case config.ProtocolGemini:
		if status < 400 {
			status = 400
		}
		return status, encodeGeminiError(msg, status), "application/json"
	default:
		return status, encodeOpenAIError(msg), "application/json"
	}
}
