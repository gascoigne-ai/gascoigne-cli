package detect

import "strings"

// Structured data.
const (
	JSON   = "data/json"
	NDJSON = "data/ndjson"
	YAML   = "data/yaml"
	CSV    = "data/csv" // and TSV
	XML    = "data/xml"
	TOML   = "data/toml"
)

func init() {
	register(
		// Before JSON: every NDJSON text is also a valid JSON stream.
		Spec{Type: NDJSON, Files: []string{"*.jsonl", "*.ndjson"}, MIME: []string{"application/x-ndjson", "application/jsonl"},
			Priority: 20, Sniff: isNDJSON},
		Spec{Type: JSON, Files: []string{"*.json"}, MIME: []string{"application/json", "application/problem+json"},
			Priority: 21, Sniff: isJSON},
		Spec{Type: YAML, Files: []string{"*.yaml", "*.yml"}, MIME: []string{"application/yaml", "application/x-yaml", "text/yaml"}, Cmds: []string{"yq"}},
		Spec{Type: CSV, Files: []string{"*.csv", "*.tsv"}, MIME: []string{"text/csv", "text/tab-separated-values"}, Priority: 84, Sniff: isCSV},
		Spec{Type: XML, Files: []string{"*.xml", "pom.xml", "*.pom", "*.xsd", "*.wsdl", "*.rss", "*.atom", "*.csproj", "*.fsproj", "*.plist"},
			MIME: []string{"application/xml", "text/xml", "application/rss+xml", "application/atom+xml", "application/soap+xml"},
			Cmds: []string{"xmllint"}, Priority: 31, Sniff: firstLine(`^\s*<\?xml `)},
		Spec{Type: TOML, Files: []string{"*.toml", "cargo.lock", "poetry.lock", "uv.lock", "pdm.lock", "pixi.lock"}, MIME: []string{"application/toml"}},
	)
}

// isCSV: a header and rows with the same number (2+) of one separator.
func isCSV(_ string, lines []string) bool {
	var rows []string
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			rows = append(rows, l)
		}
	}
	if len(rows) < 4 {
		return false
	}
	for _, sep := range []string{",", "\t", ";"} {
		n := strings.Count(rows[0], sep)
		if n < 2 {
			continue
		}
		same := 0
		for _, r := range rows {
			if strings.Count(r, sep) == n {
				same++
			}
		}
		if same*10 >= len(rows)*9 && !strings.ContainsAny(rows[0][:1], "{[<#") {
			return true
		}
	}
	return false
}
