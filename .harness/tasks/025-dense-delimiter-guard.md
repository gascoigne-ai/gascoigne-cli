status: todo
# fix(dense): skip delimited lists

`dense` treats any line of 300+ bytes under `maxSpaceRatio` as minified
(`internal/optimizer/denseoptimizer/dense.go:68-77`). A `|`- or `,`-joined list of names got
`…[2681 bytes dense]…`, which lost the content.

Scope: skip lines where a delimiter (`|`, `,`, `;`) recurs at regular density, or tokens are mostly
word-like. Keep truly minified JS/CSS/base64 folding.

Done when: fixture for a delimited list kept; adversarial goldens unchanged; `make check` and
`make eval` pass; architecture doc updated in the same commit.
