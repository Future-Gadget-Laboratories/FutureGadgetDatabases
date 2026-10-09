# Runner setup configuration

The runner setup script accepts a small YAML file with `--config`. It reads
the settings once at startup. Command-line flags override the file, and the
file overrides the built-in defaults. The commented example is
`build/fgdb/runner/config.example.yaml`.

The important storage settings are the cache path and the allowed local
filesystem types. The setup checks the directory that will actually hold the
cache, including free bytes and free inodes. The installed service checks it
again before each start.

Runner self-update remains enabled by default. The first installed runner
package is checked against `build/fgdb/runner/checksums.txt`.
