status: todo

`dmesg` / `dmesg -T` (`[12345.678] usb 1-1: …`) falls to `doc/text`: the log sniffs need ISO dates or
levels (`detect/log.go:35-36`), so only exact-duplicate folding applies. Driver messages repeat heavily.

Scope: add `Spec{Type:"log/dmesg", Cmds:["dmesg"], Sniff: ratio(`^\[\s*\d+\.\d+\] `, 0.6)}` at
`detect/log.go:20`. The template engine (`logoptimizer/log.go:109-178`) should work unchanged. Real
`dmesg` output fixture.

Done when: `make check` and `make eval` pass; matrix example present;
`.harness/architecture/families.md` log section updated in the same commit.
