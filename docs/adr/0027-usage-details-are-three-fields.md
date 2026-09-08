# Usage Details are three analogue fields

Status: accepted. Amends ADR-0007.

Conversion preserves reasoning tokens, cache-read tokens, and cache-create tokens when the other protocol has an Analogue. Prompt and completion counts stay as they are. There is no extras bag for the rest of an upstream usage object. Claude has no reasoning-token Analogue — drop. Cache-create emits only toward Claude `cache_creation_input_tokens`; other targets drop.
