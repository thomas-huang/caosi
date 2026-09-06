package convert

import (
	"encoding/json"
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
