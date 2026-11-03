#!/usr/bin/env python3
"""json protocol: one request on stdin, one response on stdout.

Keeps WARN/ERROR/FATAL lines (and the stack lines that follow them) from a
log and says what was dropped. Shows the three things a json child gets:
the block type, the attrs (cmd, file, ...) and the skip_reason answer.
"""
import json
import re
import sys

LEVEL = re.compile(r"^\S+\s+(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\b")
KEEP = {"WARN", "ERROR", "FATAL"}


def shrink(text):
    kept, dropped, keeping = [], {}, False
    lines = text.splitlines()
    for line in lines:
        m = LEVEL.match(line)
        if m:
            keeping = m.group(1) in KEEP
            if not keeping:
                dropped[m.group(1)] = dropped.get(m.group(1), 0) + 1
        if keeping:
            kept.append(line)
    if not dropped:
        return None
    gone = ", ".join(f"{n} {lvl}" for lvl, n in sorted(dropped.items()))
    return f"[log-levels] kept {len(kept)} of {len(lines)} lines; dropped {gone}\n" + "\n".join(kept) + "\n"


def main():
    req = json.load(sys.stdin)
    text = req["block"]["text"]
    out = shrink(text) if req["block"]["type"].startswith("log/") else None
    if out is None:
        resp = {"version": 1, "text": text, "skip_reason": "no log levels found"}
    else:
        resp = {"version": 1, "text": out}
    json.dump(resp, sys.stdout)


main()
