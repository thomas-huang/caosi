# Honor HTTP_PROXY and HTTPS_PROXY

caosi uses the default HTTP proxy environment (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`) for upstream calls. Local converters in restricted networks otherwise cannot reach providers. Credentials will pass through that proxy; there is no second proxy field in the Provider config.
