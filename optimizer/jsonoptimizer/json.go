// Package jsonoptimizer compresses JSON: drops empty values, samples long
// arrays keeping ends, error-like items, numeric outliers and rare values,
// and turns arrays/maps of similar objects into tables with constant columns
// and shared prefixes/suffixes hoisted out.
package jsonoptimizer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

// legend explains table encoding once per output, so readers can decode rows.
const legend = `"_table": rows align with _cols; _same: equal in all rows; _affix: value = prefix + cell + suffix`

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		values, err := decode(b.Text)
		if err != nil {
			return "", err
		}
		if b.Type == detect.NDJSON || len(values) > 1 {
			values = []any{values} // records of one stream form one array
		}
		var out bytes.Buffer
		for _, v := range values {
			if err := encode(&out, o.crush(v)); err != nil {
				return "", err
			}
		}
		if bytes.Contains(out.Bytes(), []byte(`"_cols":`)) {
			return "// " + legend + "\n" + out.String(), nil
		}
		return out.String(), nil
	}, detect.JSON, detect.NDJSON)
}

// decode reads one or more concatenated JSON values.
func decode(text string) ([]any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	var values []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return values, nil
		}
		if err != nil {
			return nil, err
		}
		values = append(values, v)
	}
}

func encode(w *bytes.Buffer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v) // compact, sorted keys, newline-terminated
}

func (o *Optimizer) crush(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			if c := o.crush(x); !o.cfg.DropEmpty || !empty(c) {
				out[k] = c
			}
		}
		return o.tabulateMap(out)
	case []any:
		items := make([]any, 0, len(v))
		for _, x := range v {
			items = append(items, o.crush(x))
		}
		return o.tabulate(o.sample(items))
	case string:
		return o.str(v)
	}
	return v
}

func empty(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}
