status: todo
# feat(infra): terraform init and state dialects

`detect/infra.go:14-18` has only plan/apply/destroy/refresh. `terraform init` prints provider download
chatter; `state list` is a long path list; `validate` and `output` are unhandled.

Scope: dialects for `init` (keep versions, errors, final status), `state list` (reuse `fs/find`
grouping), `validate`, `output`, for terraform/tofu/terragrunt. Real output fixtures.

Done when: `make check` and `make eval` pass; matrix examples present; architecture doc updated in the
same commit.
