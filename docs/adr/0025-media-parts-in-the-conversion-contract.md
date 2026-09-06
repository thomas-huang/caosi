# Media parts are in the Conversion Contract

Status: accepted. Amends ADR-0002 and ADR-0007.

Conversion preserves Image, Document, Audio, and Video Parts when the other protocol has an Analogue, including when nested in a tool result, on both request and response. Dedicated image, files, audio, video, and realtime endpoints stay out; `protocol.Detect` does not grow. A part is classified by the Client Protocol’s named block type when it has one, otherwise by MIME. Carriage is inline bytes or an http(s) URL; `file_id` / `fileUri` are dropped; caosi does not fetch URLs. No Analogue means drop that part and still send the rest (ADR-0018), with no placeholder text. Bodies over the existing hop limit still 413; the contract does not include making large inline video fit.
