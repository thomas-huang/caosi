# Conversion Contract

Status: accepted. How unrepresentable fields are handled is superseded by ADR-0018. Which capabilities are in the contract is amended by ADR-0025, ADR-0026, and ADR-0027.

Conversion must preserve text, system instruction, tools, thinking/reasoning, and image parts. Coding agents are unusable without tools; reasoning models are silently worse without thinking; image parts are in the contract because clients already embed them in chat messages even though dedicated image APIs are out of scope.

The current capability list is [CONTEXT.md](../../CONTEXT.md) (amended by ADR-0025, ADR-0026, ADR-0027). Analogue keep/drop tables: [next-version.md](../next-version.md).
