# Thinking Signature is a carriage on thinking parts

Status: accepted. Amends ADR-0007.

A Thinking Signature is an opaque blob on a thinking part, not thinking text. Conversion copies it between named Analogues on thinking parts and does not verify it. Analogues: Claude `thinking.signature`, Gemini `thoughtSignature` on a thought part, OpenAI Responses reasoning `encrypted_content`. OpenAI Chat has no Analogue — drop. A Gemini `thoughtSignature` on `functionCall` is not on a thinking part — drop. Conversion does not invent a thinking part to carry a tool-call signature, and does not stash the blob in a field the target protocol does not name.

The blob need not be valid for the Client Protocol’s vendor: Claude `signature` may hold a Gemini token so the client can echo it. Passthrough still copies the upstream’s real signature. Incremental pairs that already emit thinking emit the signature when present; this does not add incremental pairs (ADR-0020).
