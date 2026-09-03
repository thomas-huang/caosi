# Full mesh includes cells CLIProxyAPI lacks

Status: accepted. Those two cells are implemented and must not 501. The earlier “do not remap via Chat” clause is amended by ADR-0019.

v1 still Converts every pair among OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini. Where CLIProxyAPI has a translator, caosi matches that behavior and those tests. Where it has none — Claude Messages → OpenAI Responses and Gemini → OpenAI Responses — caosi still implements the cell; shrinking the grid would break Claude clients against Responses Providers.
