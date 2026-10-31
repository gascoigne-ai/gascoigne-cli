status: todo

Images are 88% of tool-result bytes in 30 days of local transcripts (418 image Reads, 137.7 MB base64).
Most are 1280x720 and 1500x850 screenshots. Nothing shrinks them without the proxy, and
`max_image_pixels` is 0 by default (`configs/default.yaml:26`). A 600k-pixel cap would save about
120-180k image tokens a month here.

Scope: reuse `receiver/httpproxyreceiver/images.go:29-55` (`shrink`, deterministic box filter) in the
PostToolUse hook, keeping the tool's output shape. `posthook.go:98-102` skips non-text blocks, and Read
is not in the matcher (`cmd/gascoigne/agents.go:155`). First verify the real `tool_response` shape the
hook receives for a Read of an image. Default `max_image_pixels` to about 600k in the proxy. Check UI
text legibility on real screenshots before shipping.

Done when: real-shape fixture; `make check` and `make eval` pass; `.harness/architecture/hooks.md`
updated in the same commit.
