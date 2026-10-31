status: todo

`table/sys` covers ps, ss, lsof, top, systemctl list, lsblk, lsmod, docker ps. Not covered: `free`,
`vmstat`, `iostat`, `lscpu`, `lspci`, `lsusb`, `ip addr|route`, `ifconfig`, `mount`/`findmnt`,
`nvidia-smi`, `systemctl status <unit>`, `env`/`printenv`.

Scope: Rows in `detect/table.go`, one command per commit, real output fixtures. `env`: cut very long
values (PATH, LS_COLORS) to head/tail only; values may be needed exactly, so keep a pointer to `raw`.

Done when: `make check` and `make eval` pass; matrix examples present; architecture doc updated in the
same commit.
