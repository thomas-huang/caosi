package livetest

import "github.com/thomas-huang/caosi/internal/config"

type cell string

const (
	cellText     cell = "text"
	cellSystem   cell = "system"
	cellTools    cell = "tools"
	cellImage    cell = "image"
	cellDocument cell = "document"
	cellAudio    cell = "audio"
	cellVideo    cell = "video"
)

var cellOrder = []cell{
	cellText, cellSystem, cellTools, cellImage, cellDocument, cellAudio, cellVideo,
}

var clientProtocols = []config.Protocol{
	config.ProtocolOpenAIChat,
	config.ProtocolOpenAIResponses,
	config.ProtocolClaudeMessages,
	config.ProtocolGemini,
}

func clientCanSend(p config.Protocol, c cell) bool {
	switch c {
	case cellText, cellSystem, cellTools, cellImage, cellDocument:
		return true
	case cellAudio:
		return p == config.ProtocolOpenAIChat || p == config.ProtocolGemini
	case cellVideo:
		return p == config.ProtocolGemini
	}
	return false
}

func upstreamHasAnalogue(p config.Protocol, c cell) bool {
	switch c {
	case cellText, cellSystem, cellTools, cellImage, cellDocument:
		return true
	case cellAudio:
		return p == config.ProtocolOpenAIChat || p == config.ProtocolGemini
	case cellVideo:
		return p == config.ProtocolGemini
	}
	return false
}

func applies(client, upstream config.Protocol, c cell) bool {
	return clientCanSend(client, c) && upstreamHasAnalogue(upstream, c)
}
