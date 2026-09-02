# Hot reload keeps the last good config

A bad save (illegal JSONC/YAML, reserved Provider Name, schema error) leaves the running process on the last config that loaded successfully and logs the error. Startup with no successful config still exits. A half-written file must not kill in-flight agent requests.
