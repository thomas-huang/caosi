package convert

import (
	"encoding/json"
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

func chatToGeminiResponse(body []byte) ([]byte, error) {
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
