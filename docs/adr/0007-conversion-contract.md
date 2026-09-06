# Conversion Contract

Status: accepted. How unrepresentable fields are handled is superseded by ADR-0018. Which capabilities are in the contract is superseded by ADR-0025.

Conversion must preserve text, system instruction, tools, thinking/reasoning, and image parts. Coding agents are unusable without tools; reasoning models are silently worse without thinking; image parts are in the contract because clients already embed them in chat messages even though dedicated image APIs are out of scope.
