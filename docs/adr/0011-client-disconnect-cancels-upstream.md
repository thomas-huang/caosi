# Client disconnect cancels the upstream

When the client closes the connection, caosi cancels the in-flight upstream request. Letting the upstream finish would keep spending tokens and quota after the user hit stop.
