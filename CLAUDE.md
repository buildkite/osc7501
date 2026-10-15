# osc7501

Go library for the OSC 7501 "program status" terminal protocol (spec rev 0.4): emit (`Reporter`), parse (`Parse`), detect in a byte stream (`Scanner`) and strip (`Stripper`) reports.

## Layout

- `osc7501.go`: package doc, limits, `Report`, `Parse`, `Encode`
- `emit.go`: states, kinds, `Query`, `Validate`, `Reporter`
- `scanner.go`: `Scanner` (io.Writer filter, callbacks)
- `strip.go`: `Stripper`
- `example_test.go`: runnable examples

## Conventions

- Run `make quality` (vet, `go mod tidy -diff`, golangci-lint, tests) before committing. `make fuzz` runs the fuzz targets.
- Tests use `testify/require` only, never `assert`.
- Conventional Commits (`feat:`, `fix:`, `docs:`, `test:`, `chore:`).
- Pre-1.0: breaking changes are allowed, bump the minor version, and must be documented in the commit message and release notes.
- Keep zero non-test dependencies.
