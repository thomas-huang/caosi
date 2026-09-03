# Module path is the GitHub origin

Status: accepted.

The Go module is `github.com/thomas-huang/caosi` so `go install github.com/thomas-huang/caosi/cmd/caosi@latest` works. A bare `module caosi` cannot be installed from the origin remote. Once anyone installs from that path, renaming it is a break; `--version` prints without reading a Provider File so a stranger can check the binary before filling in credentials.
