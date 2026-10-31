status: todo
# feat(k8s): kubectl apply and rollout dialects

`kubectl apply|delete|rollout status` print hundreds of `deployment.apps/x unchanged|configured|created`
lines or repeating progress lines. They are not in `detect/k8s.go:16-30`.

Scope: add dialect Specs. Fold by verb (`unchanged ×120 (…)`), list configured/created/deleted and any
error lines, collapse rollout progress to the final state. Real output fixtures.

Done when: `make check` and `make eval` pass; matrix example present; `.harness/architecture/families.md`
k8s section updated in the same commit.
