#!/usr/bin/env bash
# Runs the shared task loop with this repo's verification (check + eval).
exec env VERIFY_CMD="${VERIFY_CMD:-make check && make eval}" "$(dirname "$0")/../common/scripts/loop.sh" "$@"
