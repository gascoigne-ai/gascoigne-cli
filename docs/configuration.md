# Configuration

The built-in config (`configs/default.yaml`, embedded in the binary) holds every default.
Your config file is laid over it, so it only needs what you change. Change settings per run
with flags, per agent with environment variables, or for good in your file.

## Your config file

On first run gascoigne writes `$XDG_CONFIG_HOME/gascoigne/config.yaml` (usually
`~/.config/gascoigne/config.yaml`) and says so once on stderr. It lists **every setting of
every component, commented out**, with a short note on each, so the defaults apply until you
change something:

```yaml
## One engine per family. Limits keep what an agent acts on first and mark every gap.
# optimizers:
#   grep:
#     max_per_file: 15                            ## distinct match lines per file
```

### After an upgrade

New versions add, rename and drop settings. Your file only holds what you changed, so new
defaults apply on their own; the template part is what goes stale.

- **A file that sets nothing** is refreshed by itself on the next command (the old one is kept
  as `config.yaml.bak`), so new settings show up in it.
- **A file with settings** is left alone. `gascoigne config -upgrade` rewrites it on the new
  template, puts your settings below it and drops the ones this version no longer has, naming
  each. The old file stays as `config.yaml.bak`.
- **A setting this version does not know** stops every command with an error naming it and
  pointing at `config -upgrade`, instead of being ignored.


To change a setting, uncomment it together with its parent keys:

```yaml
optimizers:
  grep:
    max_per_file: 40
```

An existing file is never overwritten. Delete it to get a fresh template.

## Where config comes from

The built-in defaults, with the first file found laid over them:

1. `-config path`
2. `$GASCOIGNE_CONFIG`
3. `$XDG_CONFIG_HOME/gascoigne/config.yaml` (created on first run when neither of the above is set)

Layering rules: maps merge key by key; a value or a list you set replaces the default one
(to add a header to `http.drop_headers`, list them all); a key left empty, such as a parent
uncommented without children, keeps the default. Decoding is strict: an unknown field is an
error, so typos fail loudly.

```sh
gascoigne config                                  # print the effective config
gascoigne config -set optimizers.log.max_lines=100
```

## Per-run overrides

| Flag | Environment | Effect |
|---|---|---|
| `-set path=yaml` (repeatable) | `GASCOIGNE_SET` (`;`-separated, applied before flags) | Assign any value by path. |
| `-only a,b` | `GASCOIGNE_ONLY` | Keep only these optimizers in every chain. |
| `-disable a,b` | `GASCOIGNE_DISABLE` | Drop these optimizers from every chain. |

```sh
gascoigne exec -set optimizers.grep.max_per_file=50 -- rg TODO
gascoigne exec -set 'router.default=[git,grep]' -- git log
GASCOIGNE_SET='optimizers.doc.max_tokens=8000;optimizers.log.context=4' gascoigne wrap claude
GASCOIGNE_DISABLE=cat gascoigne wrap codex
```

Values are YAML, so lists and maps work: `-set 'optimizers.doc.man_drop=[SEE ALSO,BUGS]'`.

Per payload, `-attr k=v` sets attributes optimizers read:

| Attribute | Effect |
|---|---|
| `cmd`, `file`, `url`, `mime` | Detection hints (normally set by `exec`, `fetch`, `compress`). |
| `query` | What you are looking for; steers sampling and page sections. |
| `raw=true` | Bypass optimization. |
| `cat_mode=full\|clean\|outline` | Source file mode for this payload. |
| `hint=top` | Print the retrieve hint before the text (used before `| head`). |

## File structure

```yaml
extensions:   { <type>: {…} }          # shared services (the ledger)
receivers:    { <type>: {…} }          # where input comes from
filters:      { <type>: {…} }          # continue or bypass, tag content types
router:
  default: [<optimizer>, …]            # chain when no route matches
  routes:  [ … ]                       # optional overrides per command/content
optimizers:   { <type>: {…} }          # one entry per optimizer used
exporters:    { <type>: {…} }          # where results go
experimental: [ … ]                    # not stable yet: prompts, continuity, or all
pipelines:
  <name>:
    receivers: [ … ]
    filters:   [ … ]
    exporters: [ … ]
    router:    { … }                   # optional, overrides the top-level router
```

Component ids are `type` or `type/name` (two differently configured instances of one type).

### Built-in pipelines

| Pipeline | Used by | Receivers → exporters |
|---|---|---|
| `cli` | `gascoigne run` | stdin → stdout |
| `exec` | `gascoigne exec` | exec → stdout |
| `hook` | `gascoigne hook codex` | hook → (result back to the agent) |
| `mcp` | `gascoigne mcp` | mcp → (tool results) |
| `mcp-proxy` | `gascoigne mcp-proxy` | mcp_proxy → (tool results) |

All use the filters `bypass, detect, minsize, history`.

### Routing

Optimizers select the blocks whose detected type they understand, so one default chain
serves every input:

```yaml
router:
  default: [task, test, build, git, grep, ls, json, data, log, cat, pkg, table, db, k8s, cloud, infra, http, html, doc]
```

Order matters only where engines hand off: `html` turns pages into markdown, then `doc`
shrinks the markdown; `task` composes test/build/pkg sections before they are seen alone.
Routes are for exceptions. The first matching route picks the chain:

```yaml
router:
  routes:
    - name: big-logs
      match: { content: "log/*", min_tokens: 2000 }   # glob over block types; any block matches
      optimizers: [log]
    - name: terraform-raw
      match: { attrs: { cmd: "terraform plan*" } }     # globs over payload attributes
      optimizers: [passthrough]                         # leave the output as it is
  default: [task, test, build, git, …]
```

`match` also takes `kind` (`cmd_output`, `tool_result`, `message`, `json`). The chosen route
is recorded per event, so `gain -by route` shows what each one did.

Routes you add replace the `routes` list as a whole, like any list.

## Component options

### Extensions

| Option | Default | Meaning |
|---|---|---|
| `ledger.path` | `$XDG_DATA_HOME/gascoigne/ledger.db` | sqlite database; env vars are expanded. `GASCOIGNE_LEDGER` overrides it. |
| `ledger.raw_ttl` | `24h` | How long originals stay retrievable. `0` keeps them forever. Events and savings are never pruned. |

### Receivers

| Option | Default | Meaning |
|---|---|---|
| `stdin.kind`, `exec.kind`, `hook.kind` | `cmd_output` | Payload kind of the input block. |
| `mcp.fetch_timeout` | `20s` | Time limit for one `fetch`. |
| `mcp.fetch_max_bytes` | `5242880` | Response bytes read by `fetch`. |
| `mcp.retrieve` | `retrieve` | Name of the tool returning originals. |
| `mcp_proxy.command` | — | Downstream server command; the CLI passes it after `--` instead. |
| `mcp_proxy.retrieve` | `gascoigne_retrieve` | Added tool's name; must not clash with downstream tools. |
| `mcp_proxy.compact_tools` | true | Drop titles restating names and `$schema`/`$id` from tool schemas. |
| `mcp_proxy.max_tools` | 0 | More downstream tools than this → `search_tools` + `call_tool`; 0 mirrors them all. |
| `httpproxy.compact_tools` | true | The same for the tools of proxied API requests. |
| `httpproxy.skill_code_lines` | 30 | Code blocks of invoked skill bodies longer than this keep their head and a pointer; 0 off. |
| `httpproxy.max_image_pixels` | 0 | Downscale tool-result screenshots above this many pixels (e.g. 1024000); 0 off. |

### Filters

| Option | Default | Meaning |
|---|---|---|
| `bypass.env` | `GASCOIGNE_RAW` | Variable that, when truthy, bypasses every payload. Attr `raw=true` does the same per payload. |
| `minsize.min_tokens` | `200` | Smaller payloads are not worth it and pass through. |
| `history.max_failures` | `1` | Bypass a source once it failed this many times. `gascoigne forget <key>` resets it. |
| `detect` | — | Tags each block with its `family/dialect` type. No options. |

### Optimizers

Limits that cut content always keep what an agent acts on first (failures, errors, unhealthy
rows, query matches) and mark every gap.

| Optimizer | Option | Default | Meaning |
|---|---|---|---|
| `build` | `max_frame` | 4 | Code frame lines kept per diagnostic (dialects may lower it). |
| `git` | `context_lines` | 1 | Unchanged lines kept around diff changes. |
| | `max_file_lines` | 200 | Diff lines per file; the rest becomes a marker. |
| `grep` | `max_per_file` | 15 | Distinct match lines per file (identical lines merge first). |
| | `max_files` | 100 | Files shown. |
| | `max_line_len` | 160 | Longer matched lines are cut in the middle. |
| `ls` | `sizes_up_to` | 60 | Show file sizes when a listing has at most this many files. |
| | `width` | 120 | Wrap entry lines at this width. |
| | `max_per_dir` | 12 | Files listed per directory in trees and path lists (du keeps twice as many). |
| `json` | see below | | The JSON crusher. |
| `log` | `context` | 2 | Lines kept around warnings and errors. |
| | `drop_debug` | true | Remove DEBUG and TRACE lines. |
| | `max_lines` | 300 | Cap; warnings and errors are kept first. |
| `cat` | `mode` | `auto` | `auto`, `full`, `clean`, `outline` (see [Source files](source-files.md)). |
| | `clean_min_tokens` | 800 | Auto mode leaves smaller files untouched. |
| | `outline_min_tokens` | 4000 | Auto mode outlines files at least this big. |
| | `min_body` | 4 | Shorter function bodies are kept even in outlines. |
| `data` | `max_rows`, `max_cell` | 30, 60 | CSV/TSV tables. |
| | `max_items` | 20 | Repeated XML siblings and TOML array tables. |
| | `json` | | Crusher options for YAML. |
| `table` | `max_rows` | 30 | Longer tables are sampled; unhealthy rows, anomalies, query matches and ends stay. |
| | `max_cell` | 60 | Longer cells are cut in the middle. |
| `db` | `max_rows`, `max_cell` | 30, 80 | Result sets and document lists. |
| | `plan_min_share` | 0.02 | EXPLAIN nodes under this share of the slowest node's time collapse. |
| `k8s` | `max_items` | 15 | Resource lists (`kind: List`) are sampled; unhealthy items stay. |
| | `max_rows`, `max_cell` | 30, 60 | Tables (`kubectl get`, events, `helm list`). |
| | `notes_lines` | 8 | Helm release NOTES. |
| `cloud` | `max_rows`, `max_cell` | 30, 60 | Tables (`-o table`, `gcloud … list`). |
| | `max_objects` | 30 | Object listings (`aws s3 ls`) are listed in full up to this size. |
| | `json` | | Crusher options for API responses. |
| `infra` | `max_plan_lines` | 300 | Past it, created and updated resources keep their header only; destroys and replacements stay whole. |
| | `diff_context` | 3 | Unchanged lines around `helm diff` changes. |
| `http` | `links` | true | Keep hrefs when HTML bodies become markdown. |
| | `drop_headers` | built-in list | Response headers (lowercase) that tell an agent nothing. |
| | `json` | | Crusher options for JSON bodies. |
| `html` | `links` | true | Keep hrefs as `[text](url)`; false keeps link text only. |
| `doc` | `max_code_lines` | 40 | Longer code blocks keep head and tail. |
| | `max_tokens` | 5200 | Longer markdown is fitted by whole sections, query matches first. |
| | `man_drop` | built-in list | Man page sections dropped as boilerplate. |
| `test` | `coverage_below` | 80 | Coverage reports list files under this percent; the rest are counted. |
| `data` | `max_output_lines` | 30 | Notebook cell text outputs keep head and tail; tracebacks stay. |
| `aria` | `max_items`, `head`, `tail` | 20, 10, 4 | Browser snapshots: runs of like siblings keep head, tail and query matches. |
| `trace` | `max_period`, `min_repeats` | 12, 3 | Stack-trace frame cycles up to this long, repeated this often, show once with a count. |
| | `min_goroutines` | 3 | Goroutines on one stack shown once with the others' ids. |
| `rerun` | `max_age`, `runs` | 20m, 3 | A re-run in the same session folds lines the last runs showed into a pointer. |
| | `min_lines`, `min_chars` | 4, 200 | Smallest run folded. |
| `pkg`, `task` | — | | No options. |

#### The JSON crusher (`json`, and `json:` under `data`, `cloud`, `http`)

| Option | Default | Meaning |
|---|---|---|
| `drop_empty` | true | Remove `null`, `""`, `[]`, `{}`. |
| `max_string` | 300 | Longer strings are cut in the middle. |
| `shorten_hashes` | true | Long hash/base64 tokens (checksums, ids) keep a prefix. |
| `tabulate_min` | 3 | Fewest similar objects worth a table. |
| `lossless_min_saving` | 0.3 | A lossless table is used when it saves at least this share… |
| `lossless_max_tokens` | 2000 | …and stays under this size. |
| `min_items`, `min_tokens` | 5, 200 | Arrays smaller than both are never sampled. |
| `max_items` | 15 | Sample target. |
| `head_share`, `tail_share` | 0.3, 0.15 | Share of the sample taken from each end. |
| `anomaly_std` | 2 | Numeric values beyond this many σ count as anomalies and stay. |
| `relevance_k` | 3 | Best query matches kept. |

Lossless first: similar objects become `{"_cols": […], "_rows": […]}` with constant columns in
`_same` and shared prefixes/suffixes in `_affix`. Only when that is not enough are arrays
sampled; gaps read `… N items omitted`.

### Exporters

| Option | Default | Meaning |
|---|---|---|
| `stdout.hint` | true | Add `[gascoigne: optimized A→B tokens · full: gascoigne raw <id>]` when output changed. |

### Tokenizers

Exact counts for `gascoigne tokens`, beside its estimates.

| Option | Default | Meaning |
|---|---|---|
| `tokenizers.anthropic_api_key` | `""` | Claude counts via `count_tokens`; empty uses `$ANTHROPIC_API_KEY`. Keep the file private. |
| `tokenizers.anthropic_model` | `claude-opus-5-5` | Model whose tokenizer counts. |
| `tokenizers.o200k` | true | Count with the GPT-4o family encoding (vocabulary downloaded once to the cache dir). |

## Example: a personal config

Only what differs from the defaults; everything else comes from the built-in config:

```yaml
# ~/.config/gascoigne/config.yaml
extensions:
  ledger: { raw_ttl: 168h }          # keep originals for a week
filters:
  minsize: { min_tokens: 400 }
  history: { max_failures: 2 }
optimizers:
  log: { context: 4, drop_debug: false, max_lines: 500 }
  doc: { max_code_lines: 80, max_tokens: 6000 }
```

`gascoigne config` prints the result.
