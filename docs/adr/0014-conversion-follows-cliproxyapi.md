# Conversion follows CLIProxyAPI as a reference, not a dependency

Field mapping for Conversion follows CLIProxyAPI's translators (https://github.com/router-for-me/CLIProxyAPI), not newAPI and not a novel policy. caosi reimplements the four-protocol pairs; `tmp/CLIProxyAPI` is a reference tree. CPA tests for OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini are the oracle where they exist. CLIProxyAPI has no Claude Messages → OpenAI Responses or Gemini → OpenAI Responses request translators; those two cells are first-party caosi work with caosi tests. caosi does not import the CPA module, does not vendor `internal/`, and does not take Codex, Antigravity, Interactions, OAuth, or CLI fingerprinting.

Unknown and out-of-contract fields are dropped on Conversion and kept on Passthrough. Tools and image parts are remapped and sent; the converter does not fail just because the upstream model might reject them. Thinking maps when an analogue exists; unmatched sub-parameters are dropped. Upstream errors still translate into the Client Protocol.

CPA is not the oracle for Image, Document, Audio, or Video Parts. Do not emit Chat `video_url`, Responses `input_audio`, or placeholder text.
