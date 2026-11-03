# External optimizers

Run your own program as an optimizer. Contract and config: [docs/external-optimizer.md](../../docs/external-optimizer.md).

| Example | Protocol | Shows |
|---|---|---|
| `strip-ansi.sh` | text | a shell filter: stdin to stdout |
| `log-levels.py` | json | block type, attrs, `skip_reason` |
| `rpc-squash.py` | rpc | a long-lived child: handshake, ids, errors |
| (none, `jq` directly) | text | any installed tool is an optimizer |

Put these in **your** config (`$XDG_CONFIG_HOME/gascoigne/config.yaml` or `-config`), not in a
repo's `.gascoigne.yaml`: external optimizers are refused there. Use absolute paths.

```yaml
optimizers:
  external/ansi:
    command: [/path/to/strip-ansi.sh]
    protocol: text
    types: [test/*]
  external/levels:
    command: [python3, /path/to/log-levels.py]
    protocol: json
    types: [log/*]
  external/squash:
    command: [python3, /path/to/rpc-squash.py]
    protocol: rpc
  external/jq:                      # no script at all
    command: [jq, -c, "del(..|nulls)"]
    protocol: text
    types: [json]

router:
  default: [external/ansi, external/levels, embedjson, trace, task, test, build, git, grep, ls, json, data, log, cat, pkg, table, db, k8s, cloud, infra, http, html, aria, websearch, doc, prose, dense, paths, truncate, rerun, external/squash]
```

Order matters: an external optimizer sees what the ones before it left.
