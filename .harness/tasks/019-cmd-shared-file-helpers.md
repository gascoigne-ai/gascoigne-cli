status: todo
kind: refactor
commit: refactor(cmd): share JSON save, backup-write and home helpers
scope: cmd/gascoigne/**

# One helper per repeated file operation in cmd/gascoigne

Real copies found by the audit:
- Atomic JSON save (MkdirAll 0o700, MarshalIndent, write `.tmp` 0o600, Rename): `prefs.go:93`,
  `installs.go:85`, `release.go:71`. All three discard the marshal error. The same body also lives in
  `internal/pricing/pricing.go` and `internal/mcpauth/mcpauth.go`; leave those two (a shared lib for two is
  speculative).
- Backup then write of user config: `install.go` and `unwrap.go` use `.gascoigne-bak` and keep perms;
  `prompts.go` and `configupgrade.go` use `.bak` and fixed 0o644.
- `os.UserHomeDir()` inline in 14 files although `home()` exists in `billing.go`.
- JSON file readers: `billing.go` `readJSON`, `proxy.go` `readJSONObject`, `mcpconfig.go` `readJSONFile`.
- `SetEscapeHTML(false)` encoder: `hook.go`, `jsonedit.go`, `proxy.go`, `replay.go`.

Steps (test first, one helper at a time):
1. `saveJSON(path, value) error`, returning the marshal error. Add a test for the failing-marshal case.
2. `writeWithBackup(file, old, new)`. Keep each caller's current backup suffix and permissions: the suffix
   is user-visible, so unifying it needs approval. Parameterize, do not change.
3. Use `home()` everywhere; keep one `readJSONFile(path, target)` and derive the others.
4. One no-HTML-escape encode helper.
5. Check `pricing.go` `cmpOr` against `cmp.Or`.

Acceptance: `make check` passes (with sandboxed HOME and XDG); no user-visible file name or permission changes.
