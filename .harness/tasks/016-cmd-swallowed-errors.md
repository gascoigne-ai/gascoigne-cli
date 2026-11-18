status: todo
kind: chore
commit: fix(cmd): return or log swallowed errors
scope: cmd/gascoigne/**

# Stop swallowing errors in cmd

- `json.Unmarshal` result dropped: `continuity.go:114,130,138,150`, `prefs.go:87`, `release.go:66`.
- `data, _ := json.Marshal`: `rules.go:61`, `setup_token.go:39` (use `saveJSON`).
- `billing.go:85` `home()` drops the `UserHomeDir` error.
- Scanner loops without `Err()`: `prompts.go:360`, `continuity.go:75`, `setup_http.go:170` (a truncated transcript reads as complete).

Test first for the scanner and corrupt-state cases (`sandbox(t)`). Acceptance: `make check`.
