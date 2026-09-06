# Upstream hop does not follow redirects or decode gzip

caosi's hop does not follow 3xx, does not ask for or decode gzip, and never returns `Location` to the client: any 3xx becomes a Client Protocol 502. Go's defaults would hide redirects, rewrite encoding, and leak upstream URLs. Passthrough copies response headers minus hop-by-hop and `Set-Cookie`; Conversion synthesizes `Content-Type` (and SSE cache headers). Client query is kept only on Passthrough; Conversion adds `alt=sse` only for Gemini streams.
