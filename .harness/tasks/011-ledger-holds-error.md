status: todo
kind: bug
commit: fix(ledger): return the database error from holds
scope: extension/ledger/**

# Stop dropping the DB error in ledger holds

`extension/ledger/ledger.go:193` does `_, same, _ = l.holds(...)` and then tries a longer id, hiding a real DB failure as a miss.

Test first: a fake or closed DB makes the lookup return the error. Fix: return it with context.

Model: unchanged
