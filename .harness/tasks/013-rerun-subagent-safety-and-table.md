status: todo
kind: bug
commit: fix(rerun): scope rerun pointers to the agent that saw the output
scope: optimizer/rerunoptimizer/**, cmd/gascoigne/hook.go, cmd/gascoigne/posthook.go, testdata/eval/**

# Rerun safety for subagents, then `table`

`optimizer/rerunoptimizer/rerun.go` keys on session plus key for 20 minutes. The hook input carries no
agent id and subagents share the session id (one session shows 27 runs of one `cat` of a task brief). A
subagent can therefore be handed a pointer to output only the parent saw. This comes from reading the
code and the ledger, not from a reproduction, so step one is to reproduce it.

Steps:
1. Reproduce with a throwaway ledger: the same command run by a parent and then by a subagent in one
   session. If the hook payload has an agent or transcript identifier, add it to the key. If it has none,
   do not emit rerun pointers when subagent traffic is detected.
2. Then add `table` to the rerun `Families`. `make eval` shows 8 same-session reruns, 55k tokens.

Test first: a rerun test with two agent ids sharing a session that must not produce a pointer.

Acceptance: the reproduction is a regression test; `make check && make eval` pass. Split step 2 into its
own commit if step 1 needs more than one.

Model: unchanged
