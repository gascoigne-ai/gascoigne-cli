package detect

// Structured data.
const (
	JSON   = "data/json"
	NDJSON = "data/ndjson"
)

func init() {
	register(
		// Before JSON: every NDJSON text is also a valid JSON stream.
		Spec{Type: NDJSON, Files: []string{"*.jsonl", "*.ndjson"}, MIME: []string{"application/x-ndjson", "application/jsonl"},
			Priority: 20, Sniff: isNDJSON},
		Spec{Type: JSON, Files: []string{"*.json"}, MIME: []string{"application/json", "application/problem+json"},
			Priority: 21, Sniff: isJSON},
	)
}
