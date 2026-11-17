# Config

## Files

- `configs/default.yaml` (embedded in the binary) holds every default.
- The first config file found is laid over it: `-config` > `$GASCOIGNE_CONFIG` >
  `$XDG_CONFIG_HOME/gascoigne/config.yaml` (`cmd/gascoigne/userconfig.go` `overlay`: maps merge,
  values and lists replace, empty keys keep the default).
- `exec` and the hooks then lay `.gascoigne.yaml` over that (`withProject`): searched from the cwd up
  to the git root (nearest wins; without a git root only the cwd), same merge rules, decoded strictly
  on its own so an error names the file. The merged YAML is what the server hashes, so each repo gets
  its own warm pipeline (`server.md`). `-set`/`-only`/`-disable` still win. Not applied to eval
  (`plain`) or other commands.
- With neither flag nor env, the first run writes `configs/user.yaml` there: every setting of
  every component commented out (`##` docs, `#` settings).
- Upgrades: a later version refreshes a file that sets nothing. `config -upgrade`
  (`configupgrade.go`) rebuilds one with settings on the new template (unknown ones dropped and
  named, old file `.bak`). An unknown setting fails up front, naming the upgrade.
- Version: `Config.Version` (`version:`, `config.SupportedVersion` = 1) is the format of the file.
  Missing means 1, so files from before the key load under strict decoding. `config.CheckVersion`
  reads it from one file's bytes: `configFile` and `withProject` check each file before overlay so the
  error names it; a newer one fails, `upgradeConfig` refuses it too and drops the user's own key
  (the template's `version: 1` is the single one). `Validate` backstops `-set version=2`.
  `configs.HasSettings` skips `configs.VersionKey`, so the bare template is still "no settings".
  Overlay keeps the user's value. Bump `SupportedVersion` only with a migration in `config -upgrade`.
- Strict decoding: unknown fields fail. Tests keep the template honest: uncommented, it equals the
  defaults, and every `Config` field of every used component appears in it (a new field fails
  until documented).
- A pipeline may list `optimizers`: only those run there (the router chains keep their order, `router.Config.Only`; unlisted ids stay defined for other pipelines). Pipelines may override `router` (proxy and CLI route differently). Optimizers configured in a
  pipeline's router require a ledger extension.

- `.gascoigne.yaml` may not define `external` optimizers (they run commands): `withProject` fails naming the file (`external-optimizer.md`).

## Per-run overrides (`internal/config/override.go`)

So each agent can tune optimization without its own config file: flags on any command, or env set
once in the agent's environment. Flags win over env.

| Flag | Env | Effect |
|---|---|---|
| `-set path=yaml` (repeatable) | `GASCOIGNE_SET` (`;`-separated, applied before flags) | assign any config value: `-set optimizers.grep.max_per_file=50`, `-set router.default=[go,git]` |
| `-only a,b` | `GASCOIGNE_ONLY` | keep only these optimizers in every chain |
| `-disable a,b` | `GASCOIGNE_DISABLE` | drop these optimizers from every chain |

`gascoigne config [flags]` prints the effective config. Per payload, `-attr k=v` still sets attrs
(e.g. `cat_mode=full`).
