# Claude←Responses stream is incremental

Status: accepted. Amends ADR-0002 for this pair.

Claude Messages as Client Protocol and OpenAI Responses as Upstream Protocol convert SSE as upstream events arrive: `output_text.delta` becomes `text_delta`, reasoning summary deltas become `thinking_delta`, function-call argument deltas become `tool_use` / `input_json_delta`. Buffering the whole Responses stream then emitting a legal one-shot Claude sequence made Claude Code report the stream ended before any complete data, then retry without streaming. Remaining Conversion pairs whose upstream is not OpenAI Chat may still buffer and emit a legal Client Protocol stream.
