# Conversion Contract

Conversion must preserve text, system instruction, tools, thinking/reasoning, and image parts. Coding agents are unusable without tools; reasoning models are silently worse without thinking; image parts are in the contract because clients already embed them in chat messages even though dedicated image APIs are out of scope.

Unrepresentable fields are two-tier in principle: Contract failures fail the request; fields outside the Contract are dropped and logged. Concrete mapping follows established converters (newAPI, sub2api), not a novel policy.
