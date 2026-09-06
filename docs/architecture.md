# Architecture

caosi listens on loopback, reads the Provider File, and either Passthroughs or Converts between Client Protocol and Upstream Protocol.

Domain terms: [CONTEXT.md](../CONTEXT.md). Decisions: [adr/](adr/).

## Process

```mermaid
flowchart LR
  CLI["Claude Code / Codex / Gemini CLI"]
  CAOSI["caosi :9999 loopback"]
  PF["Provider File"]
  UP["Provider"]

  CLI -->|"/{provider_name}/..."| CAOSI
  PF -->|"hot reload"| CAOSI
  CAOSI -->|"Passthrough or Conversion"| UP
```

`app` binds loopback, loads the Provider File, and starts `server`. `/health` lists Provider Names. `health` is a Reserved Provider Name.

## Request path

```mermaid
flowchart TD
  REQ["HTTP request"]
  SPLIT["Provider Name + remaining path"]
  DET["Detect Client Protocol"]
  LOOK["Lookup Provider"]
  DEC{"Client Protocol = Upstream Protocol?"}
  REQCV["convert.Request"]
  PATH["UpstreamPath + JoinURL"]
  HDR["header allowlist + credential"]
  UP["Provider"]
  PASS["copy body and headers"]
  STREAM["convert.Stream"]
  RESP["convert.Response"]
  ERR["convert.ClientError in Client Protocol"]

  REQ --> SPLIT --> DET --> LOOK
  LOOK --> DEC
  DEC -->|yes, maybe Model Override| REQCV
  DEC -->|no| REQCV
  REQCV --> PATH --> HDR --> UP
  UP -->|Passthrough| PASS
  UP -->|Conversion + SSE| STREAM
  UP -->|Conversion + JSON| RESP
  LOOK -.->|missing Provider or protocol| ERR
  UP -.->|connect / remap failure| ERR
```

`convert.Request` also applies Model Override when the Provider has one, including on Passthrough.

## Conversion

The Conversion seam is `Request`, `Response`, `Stream`, plus `NeedsConvert`, `UpstreamPath`, `ModelFromBody`, `ClientError`. Pairwise field maps sit behind that seam.

Non-stream Conversion remaps through OpenAI Chat in-process ([ADR-0019](adr/0019-conversion-uses-chat-as-in-process-ir.md)):

```mermaid
flowchart LR
  C["Client Protocol body"]
  IR["OpenAI Chat IR"]
  U["Upstream Protocol body"]

  C -->|"toChat"| IR -->|"fromChat"| U
```

The reverse path is `Response`: upstream body → Chat IR → Client Protocol. Upstream errors become Client Protocol errors ([ADR-0008](adr/0008-errors-match-the-client-protocol.md)).

### Stream

```mermaid
flowchart TD
  S["convert.Stream"]
  SAME{"same protocol?"}
  COPY["copy bytes"]
  INC{"incremental pair?"}
  A["Claude Messages ← OpenAI Chat"]
  B["Claude Messages ← OpenAI Responses"]
  C["OpenAI Responses ← OpenAI Chat"]
  D["Gemini ← OpenAI Chat"]
  BUF["assemble complete upstream body"]
  R["convert.Response via Chat IR"]
  OUT["Client Protocol SSE or JSON"]

  S --> SAME
  SAME -->|yes| COPY
  SAME -->|no| INC
  INC -->|yes| A
  INC -->|yes| B
  INC -->|yes| C
  INC -->|yes| D
  INC -->|no| BUF --> R --> OUT
```

Claude Messages ← OpenAI Responses stays incremental ([ADR-0020](adr/0020-claude-responses-stream-is-incremental.md)). Remaining pairs may buffer, then emit a legal Client Protocol stream.

## Protocol grid

Full mesh among OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini ([ADR-0002](adr/0002-full-mesh-streaming-conversion.md), [ADR-0017](adr/0017-full-mesh-includes-cpa-holes.md)):

```mermaid
flowchart LR
  subgraph clients [Client Protocol]
    CC[OpenAI Chat]
    CR[OpenAI Responses]
    CM[Claude Messages]
    CG[Gemini]
  end
  subgraph upstreams [Upstream Protocol]
    UC[OpenAI Chat]
    UR[OpenAI Responses]
    UM[Claude Messages]
    UG[Gemini]
  end
  CC --> UC
  CC --> UR
  CC --> UM
  CC --> UG
  CR --> UC
  CR --> UR
  CR --> UM
  CR --> UG
  CM --> UC
  CM --> UR
  CM --> UM
  CM --> UG
  CG --> UC
  CG --> UR
  CG --> UM
  CG --> UG
```

Same-protocol cells are Passthrough. Cross-protocol cells are Conversion. Conversion Contract: text, system instruction, tools, thinking/reasoning, image parts.

## Packages

```mermaid
flowchart TB
  CMD["cmd/caosi"]
  APP["app — flags, loopback, first run"]
  SRV["server — /health, proxy, hot reload"]
  CFG["config — Provider File"]
  PROTO["protocol — Detect, JoinURL"]
  HDR["header — allowlist, credential"]
  CVT["convert — Request, Response, Stream"]

  CMD --> APP --> SRV
  SRV --> CFG
  SRV --> PROTO
  SRV --> HDR
  SRV --> CVT
```

| Package | Owns |
| --- | --- |
| `config` | Provider File, Provider, Protocol |
| `protocol` | Client Protocol from path; `base_url` join |
| `header` | Header allowlist by Upstream Protocol; Provider credential |
| `convert` | Passthrough Model Override; Conversion; Client Protocol errors |
| `server` | Listen path, Provider lookup, HTTP hop, hot reload |
| `app` | CLI, loopback bind, first-run template |
