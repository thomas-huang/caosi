# IR is not OpenAI Chat

Status: accepted. Amends ADR-0019.

Cross-protocol Conversion still remaps in-process through one IR, not a second HTTP hop and not twelve pairwise field maps. That IR is no longer OpenAI Chat JSON: Chat has no Analogue for Document, Audio, or Video Parts, so a Gemini PDF going to Claude was emptied in the hub. The IR holds every Conversion Contract capability; OpenAI Chat is only a Client Protocol and an Upstream Protocol.

LiteLLM’s Chat hub plus URL-fetch is rejected — fetching is a gateway. CLIProxyAPI’s pairwise JSON rewrites are rejected — Claude `document` survives to one target and dies toward another. Vercel’s single `file`+`mediaType` blob is rejected as the IR kind: caosi already named Image, Document, Audio, and Video Parts.
