# Errors match the Client Protocol

Conversion translates upstream errors into the Client Protocol error shape and keeps the status code when it can. Passthrough leaves an already-correct error body alone. Passing the upstream body through on a Conversion path, or inventing a caosi-specific error JSON, breaks client SDKs.
