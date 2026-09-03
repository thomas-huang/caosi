# Conversion uses Chat as an in-process intermediate

Status: accepted. Amends ADR-0017.

Same-protocol requests still Passthrough (ADR-0001). Cross-protocol Conversion — including Claude Messages → OpenAI Responses and Gemini → OpenAI Responses — remaps through OpenAI Chat inside the process, not as a second HTTP hop. Dedicated pairwise translators without an intermediate would duplicate the same field maps; a network hub would add latency and contradict on-demand conversion. The CPA-missing cells are implemented in-process, not 501.
