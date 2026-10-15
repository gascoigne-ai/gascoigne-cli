# Config

## Files

- `configs/default.yaml` (embedded in the binary) holds every default.
- The first config file found is laid over it: `-config` > `$GASCOIGNE_CONFIG` >
  `$XDG_CONFIG_HOME/gascoigne/config.yaml` (`cmd/gascoigne/userconfig.go` `overlay`: maps merge,
  values and lists replace, empty keys keep the default).
- With neither flag nor env, the first run writes `configs/user.yaml` there: every setting of
  every component commented out (`##` docs, `#` settings).
- Upgrades: a later version refreshes a file that sets nothing. `config -upgrade`
  (`configupgrade.go`) rebuilds one with settings on the new template (unknown ones dropped and
  named, old file `.bak`). An unknown setting fails up front, naming the upgrade.
- Strict decoding: unknown fields fail. Tests keep the template honest: uncommented, it equals the
  defaults, and every `Config` field of every used component appears in it (a new field fails
  until documented).
- Pipelines may override `router` (proxy and CLI route differently). Optimizers configured in a
  pipeline's router require a ledger extension.

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
