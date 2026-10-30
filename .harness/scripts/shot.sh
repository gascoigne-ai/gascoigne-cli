#!/usr/bin/env bash
# Screenshot a dashboard page with headless Firefox, to check a UI change by eye.
#   .harness/scripts/shot.sh <path> <out.png> [width] [height]    e.g. shot.sh /context /tmp/ctx.png 1280 2400
# Needs a running server (make build, then GASCOIGNE_SERVER_ADDR=127.0.0.1:7992 bin/gascoigne server run) and
# firefox + python3. Pages fetch their data after load, and Firefox shoots at the load event, so a small proxy
# adds a blocking script that holds the load event for 4s. Stop the old server first (`server stop`): a stale
# one keeps serving the old embedded pages.
set -euo pipefail
page=${1:?path, e.g. /context}; out=${2:?output .png}; w=${3:-1280}; h=${4:-1600}
addr=${GASCOIGNE_SERVER_ADDR:-127.0.0.1:7878}; port=${SHOT_PORT:-7993}
work=$(mktemp -d); trap 'kill $proxy 2>/dev/null; rm -rf "$work"' EXIT
cat >"$work/proxy.py" <<PY
import http.server, urllib.request, time
class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path.startswith('/slow.js'):
            time.sleep(4); self.send_response(200); self.send_header('Content-Type', 'text/javascript'); self.end_headers(); return
        r = urllib.request.urlopen('http://$addr' + self.path); b = r.read(); ct = r.headers.get('Content-Type', '')
        if 'text/html' in ct: b = b.replace(b'</body>', b'<script src="/slow.js"></script></body>')
        self.send_response(200); self.send_header('Content-Type', ct); self.end_headers(); self.wfile.write(b)
http.server.ThreadingHTTPServer(('127.0.0.1', $port), H).serve_forever()
PY
python3 "$work/proxy.py" & proxy=$!
sleep 1
# --no-remote and a throwaway profile: a running Firefox otherwise swallows the command.
firefox --headless --no-remote --profile "$work" --screenshot "$out" --window-size="$w,$h" "http://127.0.0.1:$port$page" >/dev/null 2>&1
echo "$out"
