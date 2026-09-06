# Conversion uses Chat as an in-process intermediate

Status: accepted. Amends ADR-0017. The sentence that Conversion remaps through OpenAI Chat is void; ADR-0024 replaced the IR. The rest still holds: in-process remap, not a second HTTP hop, not pairwise-only, not 501.

Same-protocol requests still Passthrough (ADR-0001). Cross-protocol Conversion — including Claude Messages → OpenAI Responses and Gemini → OpenAI Responses — remaps in-process, not as a second HTTP hop. Dedicated pairwise translators without an intermediate would duplicate the same field maps; a network hub would add latency and contradict on-demand conversion. The CPA-missing cells are implemented in-process, not 501.
