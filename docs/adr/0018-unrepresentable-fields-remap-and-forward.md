# Unrepresentable fields remap and forward

Status: accepted. Supersedes the fail-closed sentence in ADR-0007.

Conversion remaps the Conversion Contract and sends the request. Fields with no analogue are dropped. caosi does not fail a request just because the upstream model might reject tools, thinking, or images; if the upstream rejects, that error is translated into the Client Protocol (ADR-0008). Fail-closed on extra Claude Code fields would make Conversion unusable; stripping tools and still returning 200 would hide contract loss. This follows CLIProxyAPI’s remap-and-forward, not newAPI’s silent drop of thinking.
