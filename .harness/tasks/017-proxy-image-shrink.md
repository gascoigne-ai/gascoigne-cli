status: todo

# Proxy: downscale screenshot images

## Goal
Image tokens scale with pixel area; screenshots from browser tools are often oversized.

## Scope
- Image blocks in tool results above `max_pixels` (config, off by default) downscaled with
  stdlib `image` (deterministic resampling, re-encoded PNG/JPEG); the original goes to the ledger.
- Cache by content hash like text results.

## Acceptance
- Tests: byte-identical output across runs, size cap honored, small images untouched.
- `make check` green.
