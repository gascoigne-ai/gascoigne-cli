status: todo

`docker pull|push` and `docker compose pull` print layer progress (`Pulling fs layer`,
`Waiting`, `Download complete`, `Already exists` x N). No Spec covers them.

Scope: dialect as `pkgnoise` rules (Yes/No examples from real output) or a `pkg/docker` Row in
`detect/pkg.go`; keep the digest, final status and errors. Also consider `docker history`.

Done when: `make check` and `make eval` pass, `TestNoiseRulesVerify` covers the rules; architecture doc
updated in the same commit.
