# base_url is a prefix

A Provider's `base_url` is the origin plus any path prefix the upstream expects. caosi joins it with the remaining request path (after Conversion, the rewritten path) and only collapses duplicate slashes. It does not strip or invent `/v1`. Compatible gateways use too many prefix shapes for a smart join to be safe; a doubled `/v1` is a configuration error.
