# On-demand conversion

caosi forwards a request unchanged when its Client Protocol matches the Provider's Upstream Protocol, and translates only when they differ. Always running every request through a canonical internal model would tax the common same-protocol path; passthrough-only would make a multi-protocol client surface a lie.
