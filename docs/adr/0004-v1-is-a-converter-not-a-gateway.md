# v1 is a converter, not a gateway manager

caosi reads Providers, listens on loopback, Passthroughs or Converts, hot-reloads config, serves a health check, and logs requests. It does not ship a Web UI, failover, key pools, OAuth, client-config takeover, or an OS service. Those belong to a different product.
