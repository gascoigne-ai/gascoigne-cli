package tableoptimizer

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

// The shared limits are embedded: the YAML keys stay max_rows and max_cell.
func TestConfigKeys(t *testing.T) {
	var c Config
	if err := yaml.Unmarshal([]byte("max_rows: 7\nmax_cell: 12\n"), &c); err != nil {
		t.Fatal(err)
	}
	if c.MaxRows != 7 || c.MaxCell != 12 || c.Validate() != nil {
		t.Fatalf("config = %+v", c)
	}
	if (&Config{}).Validate() == nil {
		t.Fatal("zero limits accepted")
	}
}
