# Next version: media parts on the existing grid

caosi stays a loopback converter among OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini. This version does not add endpoints, storage, URL fetch, or a Files API. It makes Conversion preserve Image, Document, Audio, and Video Parts that already arrive inside those four chat protocols.

Domain language: [CONTEXT.md](../CONTEXT.md). Decisions: [ADR-0024](adr/0024-ir-is-not-openai-chat.md), [ADR-0025](adr/0025-media-parts-in-the-conversion-contract.md). Architecture: [architecture.md](architecture.md).

## Why this version

v1’s Conversion Contract listed image parts. Request-side images mostly remap; HTTP URLs into Gemini are dropped; `tool_result` images are flattened to text; assistant output is a string, so generated images vanish. Document, audio, and video parts have no cases and are dropped. Dedicated `/v1/images/*`, `/v1/files`, video-generation, and TTS paths stay 404.

Coding agents (Claude Code, Codex, Gemini CLI) already put screenshots, PDFs, and other blobs in the same four endpoints. That is contract loss, not a new product. Gemini CLI `fileUri` and Chat/Responses `file_id` are dropped; only inline bytes and http(s) URLs convert.

## Product boundary

In:

- Full mesh on the existing endpoints (ADR-0002).
- Image, Document, Audio, Video Parts on request and response, including inside tool results.
- Inline bytes and http(s) URLs.
- Drop when there is no Analogue; still send the rest.

Out:

- Dedicated image / files / audio / video / realtime / embeddings / batch / count_tokens endpoints.
- Fetching a URL to inline it.
- `file_id` / Gemini `fileUri` (those are Files API objects).
- Placeholder text such as `[video omitted]`.
- Invented OpenAI Chat fields (`video_url`) on a Chat upstream.
- Raising the 32MiB hop cap or chunking bodies.
- Web UI, OAuth, key pools, blob store.

## IR

Conversion remaps client body → IR → upstream body, and the reverse for responses. Same-protocol Passthrough still copies bytes and never enters IR.

The IR is not OpenAI Chat JSON.

```
IR
├── model, system instruction, tools, thinking config
└── messages[]
    ├── role
    └── parts[]
        ├── text
        ├── thinking
        ├── tool call / tool result (tool result may contain parts)
        └── Image | Document | Audio | Video
            ├── mime
            └── inline bytes | http(s) URL
```

Classification: if the Client Protocol names a contract kind (`image`, `document`, `input_audio`, …), that name wins; otherwise MIME (`image/*`, `audio/*`, `video/*`, else Document Part). `data:` URLs are inline bytes, not http(s). Gemini `inlineData` with `text/plain` is a Document Part, not a text part. A Chat `image_url` whose MIME is not `image/*` stays an Image Part and is dropped if the target image Analogue cannot take that MIME — do not reclassify it as a Document Part. Unofficial Chat `video_url` is ingested as a Video Part (MIME from the payload); it is never emitted to a Chat upstream.

IR stores raw bytes, MIME, optional filename, and optional audio format. Targets encode themselves (data URL vs raw base64). If the target requires a filename and the client did not send one, synthesize one from the MIME (`document.pdf`, …). IR is not Chat JSON and not a Files API.

## What to copy, what to reject

| Source | Copy | Reject |
| --- | --- | --- |
| LangChain content blocks | Typed image / video / audio / file parts with `url` **or** `base64` **or** `mime_type`. Kind is first-class. | `file_id`, `extras`, `NonStandardContentBlock` as a dumping ground. |
| Vercel AI SDK v3 | Media is data **or** URL, not an id. Runtime: not every target can take every part. Tool results may contain files. | Collapsing all four kinds into one `file`+`mediaType` in the IR. Fetching inside the converter. |
| Pydantic AI | Per-provider analogue table. Default is **send the URL**, let the provider download. List unsupported combinations instead of pretending. | `force_download`. Files API / `UploadedFile`. Degrading a document into extracted text inside the converter. |
| LiteLLM | MIME routing: PDF data URL → Claude `document`, not `image`. | Chat-as-hub. HTTP GET of image URLs into base64. In-memory image cache. |
| CLIProxyAPI (reference tree, ADR-0014) | MIME dispatch on Gemini `inlineData`. Pass HTTP URL through when the target has `source.url`. Never fetch. Do not flatten Claude `tool_result` image/document blocks to concatenated text; keep them as nested IR parts and emit only where the analogue table has a cell. | Pairwise-only maps (Claude `document` lives on one pair and dies on another). Invented Chat `video_url` toward a Chat upstream. Text placeholders. Unofficial tool-message content arrays. Mislabeling every Gemini response `inlineData` as an image. Dedicated `/v1/images` and `/v1/videos`. |
| supermemoryai/llm-bridge | Same four-protocol hub graph. Typed `text\|image\|audio\|video\|document\|tool_*\|thinking` parts with `{url,data,mimeType}`. Pass-through URLs, no fetch. | `_original` lossless round-trip (conflicts with remap-and-forward). Unknown blocks `JSON.stringify` into text. Anthropic emit maps image only, not document. Google emit requires `media.data`, so URL-only images die. |
| new-api / Bifrost / OpenRouter | OpenRouter’s chat tags (`image_url`, `file`, `input_audio`) confirm which names exist on Chat-shaped clients. | Gateway: OCR plugins, Files API, `/images` `/videos` `/audio/speech`, failover UI. new-api drops thinking on some Gemini→OpenAI paths. |

LangChain (typed kinds), Pydantic AI (analogue table), Vercel (data vs URL), llm-bridge (same four-protocol graph) are the useful IRs. LiteLLM and CLIProxyAPI show what not to do with Chat-as-hub, URL fetch, and pairwise drift.

## Analogue table

Emit only fields the **target wire protocol** already names. Empty / “drop” = drop that part (ADR-0018), do not invent. No “if the protocol has it.”

Chat `image_url` is `{type:image_url,image_url:{url}}`. Responses `input_image.image_url` is a string. Claude image URL is `source.type=url`. Chat `file.file_data` is a data URL and needs `filename`. Chat `input_audio.data` is raw base64; `format` is `wav` or `mp3` only — other audio MIME toward Chat is dropped.

| Part / carriage | OpenAI Chat | OpenAI Responses | Claude Messages | Gemini |
| --- | --- | --- | --- | --- |
| Image, inline | `image_url` data URL | `input_image` | `image` + `source.base64` | `inlineData` |
| Image, http(s) URL | `image_url` URL | `input_image` URL string | `image` + `source.url` | drop |
| Document, inline | `type:file` + `file.filename` + `file.file_data` data URL | `input_file` + `file_data` | `document` + `source.base64` | `inlineData` |
| Document, http(s) URL | drop | drop (`input_file` is `file_data` / `file_id`, not a URL; do not emit `file_url`) | `document` + `source.url` | drop |
| Audio, inline | `input_audio` (`wav`/`mp3` only) | drop | drop | `inlineData` |
| Audio, http(s) URL | drop | drop | drop | drop |
| Video, inline | drop | drop | drop | `inlineData` |
| Video, http(s) URL | drop | drop | drop | drop (YouTube/`fileUri` are Files API objects, out of contract) |
| `file_id` / `fileUri` / Claude `source.type=file` | drop | drop | drop | drop |
| Chat `audio.data` bytes | `input_audio` | drop | drop | `inlineData` |
| Chat `audio.id` only | drop | drop | drop | drop |
| Tool-result nested Image/Document | drop (tool `content` is a string) | drop (`function_call_output.output` is a string) | keep on `tool_result.content[]` | drop (`functionResponse.response` is a JSON object, not `inlineData`) |
| Response Image | drop (`message.content` is a string) | drop (`image_generation_call` is out of scope) | drop | `inlineData` + `image/*` |
| Response Audio | `message.audio.data` only | drop | drop | `inlineData` when bytes exist |
| Response Video / Document | drop | drop | drop | `inlineData` with matching MIME |

Gemini `fileData.fileUri` is not an http(s) URL Analogue; it is a Files API object, so it is dropped.

Video **emits** only toward Gemini `inlineData`. Every other target drops it. Same-protocol Gemini is Passthrough and never uses IR.

Pydantic AI’s table matches this shape: Chat and Claude have no Video Analogue; Claude has no Audio Analogue; Gemini accepts inline bytes for every MIME. Google AI Studio downloading arbitrary HTTPS is a provider fetch, which caosi will not do.

## Stream

Incremental pairs stay text / thinking / tool-call deltas (ADR-0020) and do not go through IR. Media parts appear only on `convert.Request`, `convert.Response`, and buffered `Stream`. Do not stream invented `delta.images`. Assistant Image Parts are in-contract only when the Client Protocol has a response Analogue (Gemini `inlineData`). Chat / Claude / Responses: drop.

## Body size

32MiB request/JSON-response cap stays. Inline video that does not fit is 413. Video that fits emits only as Gemini `inlineData`.

## Delivery order

1. Replace Chat JSON with IR behind `convert.Request`, `convert.Response`, and buffered `Stream` only. Incremental pairs listed in `architecture.md` do not go through IR. Stop coercing every Gemini `inlineData` into `image_url`.
2. Close Image Part holes the contract already claimed on **request**: nested parts kept in IR (emit only where the analogue table has a cell); HTTP URLs kept on Claude/Chat/Responses; Chat HTTP URL → Gemini still dropped. Assistant images only when the client is Gemini.
3. Document Parts (Claude `document`, Responses `input_file`, Gemini non-image `inlineData`, Chat `type:file` with `file_data`).
4. Audio and Video Parts per the analogue table.
5. Grid tests for each part × carriage × protocol pair, including drop cells.

Passthrough, loopback, Provider File, header allowlist, and `protocol.Detect` do not change.
