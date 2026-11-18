status: todo
kind: bug
commit: fix(exec): drain output and check scan error on failure
scope: receiver/execreceiver/**

# Fix execreceiver watch leak

`receiver/execreceiver/exec.go` `watch` returns on a `process` error without draining the pipe or waiting on `done`, so the child blocks on write and the goroutine lives until ctx ends. `sc.Err()` is never checked.

Re-verify first: reproduce with a failing process and a blocked writer. Rate the bug (frequency x severity); below 5 move it to `backlog.md` and delete this task.

Test first: a regression test with a comment saying what went wrong. Fix: `defer pr.Close()`, wait on `done`, check `sc.Err()` and log it at debug. Pass `inv` into `watch` instead of re-reading it from ctx.

Model: unchanged
