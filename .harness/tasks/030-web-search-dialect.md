status: todo
# feat(web): WebSearch dialect

`WebSearch` results stay unchanged: about 98k tokens a month locally, no dialect.

Scope: inspect real WebSearch `tool_response` shapes (hook and proxy), add a dialect that keeps
title, URL and a trimmed snippet per hit and uses `query` to rank. Verify what shape the PostToolUse
hook actually receives first.

Done when: real-shape fixture; `make check`, `make eval` and `make eval-mcp` pass; architecture doc
updated in the same commit.
