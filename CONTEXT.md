# caosi

A local converter between LLM client protocols and named upstream providers.

## Language

**Provider**:
A named upstream: a Provider Name, a base URL, a credential, extra request headers, one Upstream Protocol, and an optional Model Override.
_Avoid_: vendor, backend, 后端, frontend, 前端

**Provider Name**:
The Provider's identity and the first path segment of a request to caosi.

**Client Protocol**:
The wire protocol of a request that arrives at caosi.
_Avoid_: frontend, 前端, inbound protocol

**Upstream Protocol**:
The wire protocol caosi uses with a Provider.
_Avoid_: backend, 后端, outbound protocol

**Protocol Family**:
One of OpenAI, Claude, or Gemini. The OpenAI family includes OpenAI Chat and OpenAI Responses.

**OpenAI Chat**:
The OpenAI Chat Completions wire protocol.

**OpenAI Responses**:
The OpenAI Responses wire protocol.

**Claude Messages**:
The Anthropic Messages wire protocol.

**Gemini**:
The Gemini generateContent wire protocol.

**Passthrough**:
A request whose Client Protocol is the same as the Provider's Upstream Protocol, sent on without body translation.

**Conversion**:
Rewriting a request and its matching response between different Client and Upstream Protocols.

**Conversion Contract**:
The capabilities Conversion must preserve when the other protocol has an Analogue: text, system instruction, tools, thinking/reasoning, Image Parts, Document Parts, Audio Parts, and Video Parts. A part nested in a tool result is still that part. A capability with no Analogue is dropped; Conversion still sends the rest of the request.

**Analogue**:
A field or part kind in the other protocol that can express the same Conversion Contract capability.

**IR**:
The in-process message model Conversion remaps through. It can hold every Conversion Contract capability. It is not a wire protocol and not OpenAI Chat.
_Avoid_: OpenAI Chat IR, hub, intermediate protocol, Chat

**Image Part**:
An image carried inside a chat message on the request or the response, not a request to a dedicated image API.
_Avoid_: image API, vision, 生图, attachment, multimodal

**Document Part**:
A chat-message payload that is not text, image, audio, or video. Not a Provider File, and not a Files API object.
_Avoid_: file, attachment, Files API, 文件

**Audio Part**:
Audio carried inside a chat message, not a request to a dedicated transcription, speech, or realtime API.
_Avoid_: audio API, realtime, Whisper, TTS

**Video Part**:
Video carried inside a chat message, not a request to a dedicated video-generation API.
_Avoid_: video API, 生视频, multimodal

**Model Override**:
An optional model name on a Provider. When present, it replaces the client's model name on the upstream request; when absent, the client's model name is kept.

**Reserved Provider Name**:
A name that cannot be a Provider because caosi already uses it as a path. `health` is reserved.

**Config Directory**:
The directory that holds caosi's Provider file. Default is `.caosi` in the user's home directory; `--config-dir` may replace it.

**Provider File**:
`providers.jsonc` inside the Config Directory. JSONC: a JSON object keyed by Provider Name, with comments and trailing commas allowed.
