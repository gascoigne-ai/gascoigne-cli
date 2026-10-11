package schema

import (
	"encoding/json"
	"testing"
)

func TestCompact(t *testing.T) {
	tests := []struct {
		name, tool, in, want string
		changed              bool
	}{
		{
			name: "generated titles and markers",
			tool: "read_file",
			in: `{"$schema":"http://json-schema.org/draft-07/schema#","title":"read_fileArguments","type":"object",
				"properties":{"file_path":{"title":"File Path","type":"string"},"limit":{"title":"Max lines to read","type":"integer"}},
				"required":["file_path"],"additionalProperties":false}`,
			want:    `{"additionalProperties":false,"properties":{"file_path":{"type":"string"},"limit":{"title":"Max lines to read","type":"integer"}},"required":["file_path"],"type":"object"}`,
			changed: true,
		},
		{
			name: "nested items, defs and unions",
			tool: "search",
			in: `{"type":"object","$defs":{"Filter":{"title":"Filter","type":"object","properties":{"fieldName":{"title":"Field Name","type":"string"}}}},
				"properties":{"filters":{"type":"array","items":{"$ref":"#/$defs/Filter"}},
				"mode":{"anyOf":[{"$id":"x","type":"string"},{"type":"null"}],"title":"Mode"}}}`,
			want:    `{"$defs":{"Filter":{"properties":{"fieldName":{"type":"string"}},"type":"object"}},"properties":{"filters":{"items":{"$ref":"#/$defs/Filter"},"type":"array"},"mode":{"anyOf":[{"type":"string"},{"type":"null"}]}},"type":"object"}`,
			changed: true,
		},
		{
			name:    "nothing redundant",
			tool:    "bash",
			in:      `{"type":"object","title":"Shell","properties":{"command":{"type":"string","description":"what to run"}}}`,
			want:    `{"properties":{"command":{"description":"what to run","type":"string"}},"title":"Shell","type":"object"}`,
			changed: false,
		},
		{
			name:    "a property named title keeps its schema",
			tool:    "create_issue",
			in:      `{"type":"object","properties":{"title":{"type":"string","title":"Title"}}}`,
			want:    `{"properties":{"title":{"type":"string"}},"type":"object"}`,
			changed: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s map[string]any
			if err := json.Unmarshal([]byte(tt.in), &s); err != nil {
				t.Fatal(err)
			}
			changed := Compact(tt.tool, s)
			got, _ := json.Marshal(s)
			if string(got) != tt.want || changed != tt.changed {
				t.Fatalf("got %s (changed %v)\nwant %s (changed %v)", got, changed, tt.want, tt.changed)
			}
		})
	}
}
