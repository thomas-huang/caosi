package convert

import (
	"bytes"
	"encoding/json"
	"fmt"

	"caosi/internal/config"
)

// Supported reports whether this build can handle the pair.
func Supported(client, upstream config.Protocol) bool {
	if client == upstream {
		return true
	}
	return client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIChat
}

func NeedsConvert(client, upstream config.Protocol) bool {
	return client != upstream
}

func UpstreamPath(client, upstream config.Protocol, clientPath string) string {
	if client == upstream {
		return clientPath
	}
	if client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIChat {
		return "/v1/chat/completions"
	}
	return clientPath
}

func UnsupportedMessage(client, upstream config.Protocol) string {
	return fmt.Sprintf(
		"caosi 这一版还不能把 %s 转成 %s。当前支持：同协议透传，以及 Claude Messages → OpenAI Chat。",
		client, upstream,
	)
}

// Request rewrites a client body for the upstream. Same-protocol only applies Model Override.
func Request(client, upstream config.Protocol, body []byte, model string, stream bool) ([]byte, error) {
	if client == upstream {
		return applyModel(body, model)
	}
	if client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIChat {
		return ClaudeToOpenAIChat(body, model, stream)
	}
	return nil, fmt.Errorf("%s", UnsupportedMessage(client, upstream))
}

// Response rewrites an upstream body for the client.
func Response(client, upstream config.Protocol, body []byte) ([]byte, error) {
	if client == upstream {
		return body, nil
	}
	if client == config.ProtocolClaudeMessages && upstream == config.ProtocolOpenAIChat {
		return OpenAIChatToClaude(body)
	}
	return nil, fmt.Errorf("%s", UnsupportedMessage(client, upstream))
}

func applyModel(body []byte, model string) ([]byte, error) {
	if model == "" || len(bytes.TrimSpace(body)) == 0 {
		return body, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return body, nil
	}
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	m["model"] = b
	out, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return out, nil
}
