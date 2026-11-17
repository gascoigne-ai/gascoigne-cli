status: todo
kind: feature
commit: feat(config): config files carry a version
scope: internal/config/**, configs/**, cmd/gascoigne/configupgrade*.go, cmd/gascoigne/userconfig*.go, docs/configuration.md, .harness/architecture/config.md

# Version key in default.yaml and user.yaml

`configs/default.yaml` and `user.yaml` have no version, so `config -upgrade` cannot tell what it upgrades. Add `version: 1` and use it in the upgrade path. Engine code: approved by the user.

## Model
`Version int` on the config root. A missing key means 1 so existing user files load under `KnownFields(true)`. A version above the supported one fails with an error naming the file. The overlay merge keeps the user's value. Access pattern: read once at load; no collection.

## Acceptance
- Test first: the tests that state the behavior, run and seen failing for the expected reason. Table test with version missing, 1 and 2.
- `make check` passes (the task touches code, so the check runs).
