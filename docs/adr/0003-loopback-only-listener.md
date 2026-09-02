# Loopback-only listener

caosi binds only loopback (`127.0.0.1` or `::1`). The port defaults to 9999 and may be overridden with `--port`. Listening on all interfaces would expose Provider credentials and the converter on the LAN; a fixed port would fail when 9999 is taken.
