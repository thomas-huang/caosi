# Full-mesh streaming conversion

v1 converts every pair among OpenAI Chat, OpenAI Responses, Claude Messages, and Gemini, on both Passthrough and Conversion, including SSE. The in-scope endpoints are Chat Completions, Responses, Messages, and generateContent / streamGenerateContent. A smaller grid would make some client/provider combinations fail at random; non-streaming Conversion would make coding agents unusable. Embeddings, image APIs, audio, files, batches, and count_tokens stay out.
