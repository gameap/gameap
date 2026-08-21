# Rust plugin backward-compatibility tests

This package verifies that WebAssembly plugins built by the Rust test-plugins
project keep working across the panel versions currently supported. The
fixtures are gzipped `.wasm` modules in `testdata/`, built in the sibling
`../test-plugins` repository and committed here; the tests load them through
`plugin.Manager` and assert that each plugin either loads and answers calls
or is rejected with a clear error.

The package is self-contained and compiles against panel **v4.3.5 and
newer**, so CI can overlay it onto release checkouts: the
`.github/workflows/plugin-compat.yaml` matrix (`HEAD`, `v4.4.1`, `v4.3.5`)
copies this directory from the triggering commit into the tagged tree and
runs `go test -race -v -count=1 ./pkg/plugin/compatrust/...`.

## Version-specific files

| File | Runs on | Purpose |
|---|---|---|
| `stubs_test.go` | v4.3.5+ | Host-module stubs shared by every panel version |
| `compat_test.go` | v4.3.5+ | Fixtures that must load and work on every supported panel |
| `stubs_v44_test.go` | v4.4+ only | Stubs for host modules added in panel 4.4 — dropped on v4.3.5, where they would not compile |
| `compat_v44_test.go` | v4.4+ only | Fixtures importing 4.4 modules must load on 4.4+ |
| `compat_v43only_test.go` | v4.3.5 only | Fixtures importing 4.4 modules must be **rejected** by 4.3.x |

The workflow deletes the files that do not apply to the matrix leg under
test, so these files must never reference each other across version groups —
each leg compiles as if the removed files had never existed.

## Adding a new wasm fixture

1. Build the plugin in the `../test-plugins` repository: `make build`.
2. Copy the artifact: `cp ../test-plugins/dist/<name>.wasm.gz testdata/`.
3. Reference the fixture from the test file matching its host-module imports:
   - `compat_test.go` — loads on every supported panel;
   - `compat_v44_test.go` — loads only on 4.4+;
   - `compat_v43only_test.go` — must be rejected by 4.3.x.
4. Run `go test -race -count=1 ./pkg/plugin/compatrust/...` locally and make
   sure the **Plugin Compatibility** workflow stays green on every matrix
   leg before merging.
