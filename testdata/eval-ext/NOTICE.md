# Dataset notice

The cases in `needle/`, `probe/` and `tool/` are derived from the evaluation fixtures of
Headroom (https://github.com/chopratejas/headroom, commit 0126fba, `headroom/evals/`),
Copyright 2025 Headroom Contributors, licensed under the Apache License 2.0
(https://www.apache.org/licenses/LICENSE-2.0).

- `needle/`, `probe/`: output of the generators `generate_ccr_test_cases(50)` and
  `generate_info_retention_cases(30)` in `runners/compression_only.py`, written one JSON file per case.
- `tool/`: the built-in samples of `load_tool_output_samples()` in `datasets.py`, one file per sample.
- `cases.yaml`: written for this repository. `keep` lists are the needles, probe facts and expected
  answers from the source cases.

Changes: split into files, each case given a `keep` list, and the repository name in `tool/github-search.json`
replaced with a neutral one. `golden/` and `baseline.tsv` are gascoigne's own output.

Apache License 2.0 section 4 applies to these files: this notice states the changes, and the license text is at
the URL above.
