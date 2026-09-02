# --config-dir overrides the Config Directory

The default Config Directory is `~/.caosi` (`%USERPROFILE%\.caosi` on Windows). `--config-dir` may replace the directory; the Provider File name inside it stays `providers.jsonc`. A single-file `--config` flag is out: hot reload, the first-run template, and the reserved-name check all assume a directory.
