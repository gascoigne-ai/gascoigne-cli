#!/usr/bin/env python3
"""rpc protocol: a long-lived child, one json object per line each way.

Use it when starting the runtime is slow (a JVM, a model, a big import):
gascoigne starts the child once and keeps it. This one collapses runs of
identical lines into "line (xN)". Replace squash() with your own work.
"""
import json
import sys


def squash(text):
    out, prev, n = [], None, 0
    for line in text.split("\n") + [None]:
        if line == prev:
            n += 1
            continue
        if prev is not None:
            out.append(prev if n == 1 else f"{prev} (x{n})")
        prev, n = line, 1
    return "\n".join(out)


def main():
    for raw in sys.stdin:
        req = json.loads(raw)
        if "hello" in req:  # handshake: answer with the version you speak
            reply = {"version": 1}
        else:
            try:
                reply = {"version": 1, "id": req["id"], "text": squash(req["block"]["text"])}
            except Exception as e:  # report, do not die: the next request still works
                reply = {"version": 1, "id": req.get("id", 0), "error": str(e)}
        sys.stdout.write(json.dumps(reply) + "\n")
        sys.stdout.flush()  # without this gascoigne waits until its timeout


main()
