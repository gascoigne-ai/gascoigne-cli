// Package jsonoptimizer compresses JSON. Values: empty ones dropped, long
// strings and hashes shortened. Arrays: lossless first (similar objects as a
// table with constant columns and shared prefixes/suffixes hoisted out);
// when that is not enough, scored sampling keeps errors, anomalies, change
// points, query matches and both ends, and marks what was omitted.
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
	c := &crusher{cfg: o.cfg, query: terms(p.Attrs[payload.AttrQuery])}
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
			if err := encode(&out, c.crush(v)); err != nil {
				return "", err
			}
		}
		if bytes.Contains(out.Bytes(), []byte(`"_cols":`)) {
			return "// " + legend + "\n" + out.String(), nil
		}
		return out.String(), nil
	}, detect.JSON, detect.NDJSON)
}

// crusher compresses one payload; query comes from its attrs.
type crusher struct {
	cfg   *Config
	query []string
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

func (c *crusher) crush(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range v {
			if y := c.crush(x); !c.cfg.DropEmpty || !empty(y) {
				out[k] = y
			}
		}
		return c.tabulateMap(out)
	case []any:
		items := make([]any, 0, len(v))
		for _, x := range v {
			items = append(items, c.crush(x))
		}
		return c.array(items)
	case string:
		return c.str(v)
	}
	return v
}

// array keeps every item when a lossless table is enough; otherwise samples.
func (c *crusher) array(items []any) any {
	raw := size(items)
	small := len(items) <= c.cfg.MaxItems || len(items) < c.cfg.MinItems || raw/4 < c.cfg.MinTokens
	// A table rarely saves more than 80%: skip trying when it could not fit.
	if small || raw/5/4 <= c.cfg.LosslessMaxTokens {
		if t, ok := c.table(items); ok {
			saved := 1 - float64(size(t))/float64(raw)
			if saved >= c.cfg.LosslessMinSaving && (small || size(t)/4 <= c.cfg.LosslessMaxTokens) {
				return t
			}
		}
	}
	if small {
		return items
	}
	kept := c.sample(items)
	if t, ok := c.table(kept); ok && size(t) < size(kept) {
		return t
	}
	return kept
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
