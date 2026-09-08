# Conversion Contract (frozen)

Loopback converter. Four protocols. Analogue tables below are the oracle. P1–P5 shipped.

Domain language: [CONTEXT.md](../CONTEXT.md). Architecture: [architecture.md](architecture.md). ADR-0024–0027.

**In:** full mesh; media Parts; Thinking Signature as thinking-part carriage; Usage Details (reasoning / cache-read / cache-create); drop when no Analogue.

**Out:** new endpoints, URL fetch, Files API, gateway, shims, extras bag, invented thinking for `functionCall`, speculative incremental pairs, live CLI in `make test`.

## Backlog

Do not grow the contract because another project listed more fields. Order is the schedule; a later row does not start because an earlier row is boring.

| # | Item | When |
| --- | --- | --- |
| **B0** | Bugs in this contract (drop cells, Signature carriage, Usage Details, round-trip, inflation) | Always first |
| **B1** | Gemini `thoughtSignature` on `functionCall` | Only if Claude Code × Gemini 3 × tool 400s or retries. Needs a new ADR. Do not invent a thinking block; do not add an extras bag. |
| **B2** | A fifth incremental stream pair | Only with an ADR-0020-class empty-stream failure on a named client |
| **B3** | Citations | Not scheduled (not a CLI break) |
| **B4** | Block-level cache markers | Not scheduled. Auto `cache_control` stays rejected. |
| **B5** | MCP / custom tools | Not scheduled (Responses-only Analogue) |

## IR

Conversion remaps client body → IR → upstream body, and the reverse for responses. Same-protocol Passthrough still copies bytes and never enters IR.

The IR is not OpenAI Chat JSON.

```
IR
├── model, system instruction, tools, thinking config
├── Usage Details on the response (reasoning / cache-read / cache-create tokens)
└── messages[]
    ├── role
    └── parts[]
        ├── text
        ├── thinking (+ optional Thinking Signature)
        ├── tool call / tool result (tool result may contain parts)
        └── Image | Document | Audio | Video
            ├── mime
            └── inline bytes | http(s) URL
```

Classification: if the Client Protocol names a contract kind (`image`, `document`, `input_audio`, …), that name wins; otherwise MIME (`image/*`, `audio/*`, `video/*`, else Document Part). `data:` URLs are inline bytes, not http(s). Gemini `inlineData` with `text/plain` is a Document Part, not a text part. A Chat `image_url` whose MIME is not `image/*` stays an Image Part and is dropped if the target image Analogue cannot take that MIME — do not reclassify it as a Document Part. Unofficial Chat `video_url` is ingested as a Video Part (MIME from the payload); it is never emitted to a Chat upstream.

IR stores raw bytes, MIME, optional filename, and optional audio format. Targets encode themselves (data URL vs raw base64). If the target requires a filename and the client did not send one, synthesize one from the MIME (`document.pdf`, …). IR is not Chat JSON and not a Files API.

A Thinking Signature is an opaque string on a thinking part. Conversion does not parse it. Chat `reasoning_content` stays thinking text, not a Thinking Signature.

## What to copy, what to reject

| Source | Copy | Reject |
| --- | --- | --- |
| LangChain content blocks | Typed image / video / audio / file parts with `url` **or** `base64` **or** `mime_type`. Kind is first-class. | `file_id`, `extras`, `NonStandardContentBlock` as a dumping ground. |
| Vercel AI SDK v3 | Media is data **or** URL, not an id. Runtime: not every target can take every part. Tool results may contain files. | Collapsing all four kinds into one `file`+`mediaType` in the IR. Fetching inside the converter. |
| Pydantic AI | Per-provider analogue table. Default is **send the URL**, let the provider download. List unsupported combinations instead of pretending. | `force_download`. Files API / `UploadedFile`. Degrading a document into extracted text inside the converter. |
| LiteLLM | MIME routing: PDF data URL → Claude `document`, not `image`. | Chat-as-hub. HTTP GET of image URLs into base64. In-memory image cache. |
| CLIProxyAPI (reference tree, ADR-0014) | MIME dispatch on Gemini `inlineData`. Pass HTTP URL through when the target has `source.url`. Never fetch. Do not flatten Claude `tool_result` image/document blocks to concatenated text; keep them as nested IR parts and emit only where the analogue table has a cell. | Pairwise-only maps (Claude `document` lives on one pair and dies on another). Invented Chat `video_url` toward a Chat upstream. Text placeholders. Unofficial tool-message content arrays. Mislabeling every Gemini response `inlineData` as an image. Dedicated `/v1/images` and `/v1/videos`. |
| supermemoryai/llm-bridge | Same four-protocol hub graph. Typed `text\|image\|audio\|video\|document\|tool_*\|thinking` parts with `{url,data,mimeType}`. Pass-through URLs, no fetch. | `_original` lossless round-trip (conflicts with remap-and-forward). Unknown blocks `JSON.stringify` into text. Anthropic emit maps image only, not document. Google emit requires `media.data`, so URL-only images die. |
| llm-rosetta | Dedicated IR (not Chat-as-hub). Analogue tables. Semantic A→IR→B→IR→A. Stream inflation (content-bearing deltas, not lifecycle envelopes). Thinking Signature as a named thinking-part field. Usage cache/reasoning counts as named fields. | Gateway / admin / embeddings / rerank. Google Interactions as a fifth protocol. Vendor shims. `provider_passthrough` / `metadata_mode`. Image placeholders and auto `cache_control`. Typed stream IR. Live CLI as default tests. |
| new-api / Bifrost / OpenRouter | OpenRouter’s chat tags (`image_url`, `file`, `input_audio`) confirm which names exist on Chat-shaped clients. | Gateway: OCR plugins, Files API, `/images` `/videos` `/audio/speech`, failover UI. new-api drops thinking on some Gemini→OpenAI paths. |

LangChain (typed kinds), Pydantic AI (analogue table), Vercel (data vs URL), llm-bridge (same four-protocol graph) are the useful IRs. llm-rosetta is the source for test shape and for Thinking Signature / Usage Details as named contract rows. LiteLLM and CLIProxyAPI show what not to do with Chat-as-hub, URL fetch, and pairwise drift.

## Analogue table — media

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
| Video, inline | drop (never emit `video_url`) | drop | drop | `inlineData` |
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

## Analogue table — Thinking Signature

Carriage only: copy the opaque blob onto the target thinking-part field. Do not verify. Cryptographic formats are not Analogues of each other; the named field is. Passthrough still copies the real upstream signature.

| | OpenAI Chat | OpenAI Responses | Claude Messages | Gemini |
| --- | --- | --- | --- | --- |
| Thinking Signature | drop | reasoning `encrypted_content` | `thinking.signature` | `thoughtSignature` on a **thought** part |
| Gemini `thoughtSignature` on `functionCall` | drop | drop | drop | drop (Conversion; Passthrough keeps bytes) |

Chat `reasoning_content` is thinking text, already in contract, not a Thinking Signature.

Claude Code × Gemini 3 × tool can still break: the signature lives on `functionCall`, which has no thinking-part Analogue. That hole is accepted. Do not invent a thinking block to hold it.

## Analogue table — Usage Details

Prompt and completion counts already remap. These three are the closed set. No extras dictionary.

| | OpenAI Chat | OpenAI Responses | Claude Messages | Gemini |
| --- | --- | --- | --- | --- |
| Reasoning tokens | `completion_tokens_details.reasoning_tokens` | `output_tokens_details.reasoning_tokens` | drop | `usageMetadata.thoughtsTokenCount` |
| Cache-read tokens | `prompt_tokens_details.cached_tokens` | `input_tokens_details.cached_tokens` | `cache_read_input_tokens` | `usageMetadata.cachedContentTokenCount` |
| Cache-create tokens | drop | drop | `cache_creation_input_tokens` | drop |

## Stream

Incremental pairs stay text / thinking / tool-call deltas (ADR-0020) and do not go through IR. Media parts appear only on `convert.Request`, `convert.Response`, and buffered `Stream`. Do not stream invented `delta.images`. Assistant Image Parts are in-contract only when the Client Protocol has a response Analogue (Gemini `inlineData`). Chat / Claude / Responses: drop.

Pairs that already emit thinking emit a Thinking Signature when present. This version does not add incremental pairs. Inflation is measured only on: Claude Messages ← OpenAI Chat, Claude Messages ← OpenAI Responses, OpenAI Responses ← OpenAI Chat, Gemini ← OpenAI Chat — content-bearing deltas, not lifecycle envelopes.

## Body size

32MiB request/JSON-response cap stays. Inline video that does not fit is 413. Video that fits emits only as Gemini `inlineData`.

P1–P5 shipped (media analogue grid, Thinking Signature, Usage Details, A→B→A, content-delta inflation). Passthrough, loopback, Provider File, header allowlist, and `protocol.Detect` do not change.
