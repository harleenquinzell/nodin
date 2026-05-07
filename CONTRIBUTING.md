# Contributing to nodin

## Adding a new block type

1. Add a fixture pair in `internal/convert/testdata/`:
   - `<type>.notion.json`: the Notion API block JSON
   - `<type>.md`: the expected markdown output
2. Implement the pull direction in `internal/convert/pull.go` (add a case to `pullBlock`)
3. Implement the push direction in `internal/convert/push.go` (add a case to `parseBlock`)
4. Verify `go test ./internal/convert/...` passes, including the round-trip invariant test
5. Update the block coverage table in `docs/plan.md`

## Running tests

```sh
make test              # unit tests, no network required
make test-integration  # integration tests against a real Notion workspace
```

For integration tests, copy `.env.example` to `.env` and fill in `NODIN_TEST_TOKEN` (a Notion integration token) and `NODIN_TEST_PAGE_ID` (a page the token can freely write under). The Makefile sources `.env` automatically. Tests create and clean up their own pages prefixed `nodin-test-` and are safe to run repeatedly.

## Coding conventions

- No testify; stdlib `testing` + `t.Helper()` only.
- `fmt.Errorf("%s: %w", op, err)` for error wrapping.
- Tokens are never logged. Do not add `fmt.Println` or `log` calls that include
  config values without redacting `Token`.
- All file writes are atomic (temp file + `os.Rename`).
- New packages must have a `_test.go` with at least one test before merging.

## Commit style

```
area: short imperative description

Optional longer body explaining why.
```

Areas: `convert`, `notion`, `sync`, `cli`, `state`, `merge`, `docs`, `ci`.
