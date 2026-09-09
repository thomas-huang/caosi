# Live Conversion Contract suite is opt-in

`make test` stays hermetic: httptest fakes, no Provider hop. A separate `make live` (`go test -tags live`) starts caosi in-process against a snapshot Config Directory (`~/.caosi-live`, overridable by `CAOSI_LIVE_CONFIG_DIR`) and sends one bundled HTTP request per Provider × Client Protocol × {json, stream}.

The first shot is a bundle: system instruction, tools, user text, and every inline media Part that the Client Protocol can express and the Upstream Protocol has an Analogue for. Stream and non-stream stay two requests. If that shot fails (except 401/403, which fail the Provider), the suite re-runs one request per cell so a 4xx names the feature. If every cell then passes, the bundle still fails — the combination was the problem.

Live does not inspect the upstream request body — no recording proxy, no body logs. Drop, Analogue tables, Thinking Signature, and Usage Details stay in unit tests. A live pass is 2xx plus a parseable Client Protocol body; model text is not scored. 401/403 fail that Provider. 429, 5xx, and timeouts retry with backoff, then fail.

A missing Upstream Protocol is a skip, not a stop: the suite runs the Providers that are present. Placeholder keys are skipped the same way. After the run it writes `live-report.md` and `live-report.html` next to the snapshot.

Live CLI clients, CI secrets, and running this suite from untagged `go test ./...` are out: they cannot stay unattended without spending keys on every machine.
