package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// object is a JSON object that keeps its key order, so editing a user's
// config file changes only what was removed.
type object struct {
	keys []string
	vals map[string]any
}

func (o *object) get(k string) any { return o.vals[k] }

func (o *object) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	o.keys = slices.DeleteFunc(o.keys, func(x string) bool { return x == k })
}

func (o *object) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

// parseJSON decodes data keeping object key order and number text.
func parseJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, errors.New("trailing data after JSON value")
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		o := &object{vals: map[string]any{}}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			o.set(k.(string), v)
		}
		_, err := dec.Token() // }
		return o, err
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token() // ]
		return arr, err
	}
	return tok, nil
}

// formatJSON encodes v indented by two spaces, without HTML escaping
// ("a && b" stays readable).
func formatJSON(v any) ([]byte, error) {
	var compact bytes.Buffer
	if err := writeValue(&compact, v); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func writeValue(b *bytes.Buffer, v any) error {
	switch v := v.(type) {
	case *object:
		b.WriteByte('{')
		for i, k := range v.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeValue(b, k); err != nil {
				return err
			}
			b.WriteByte(':')
			if err := writeValue(b, v.vals[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, x := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeValue(b, x); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		enc := json.NewEncoder(b)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return fmt.Errorf("encode %T: %w", v, err)
		}
		b.Truncate(b.Len() - 1) // Encode ends with a newline
	}
	return nil
}

// path walks nested objects by key; nil when a step is missing.
func path(v any, keys ...string) any {
	for _, k := range keys {
		o, ok := v.(*object)
		if !ok {
			return nil
		}
		v = o.get(k)
	}
	return v
}

// pruneEmpty deletes keys of o whose value became an empty object or list.
func pruneEmpty(o *object, keys ...string) {
	for _, k := range keys {
		switch v := o.get(k).(type) {
		case *object:
			if len(v.keys) == 0 {
				o.del(k)
			}
		case []any:
			if len(v) == 0 {
				o.del(k)
			}
		}
	}
}

// removeHooks drops hook handlers whose command is ours from the event list
// at hooks.<event>: flat entries ({"command": …}, cursor) and matcher groups
// ({"hooks": [{"command": …}]}, claude). Returns how many went.
func removeHooks(root *object, event string, ours func(cmd string) bool) int {
	hooks, ok := root.get("hooks").(*object)
	if !ok {
		return 0
	}
	entries, ok := hooks.get(event).([]any)
	if !ok {
		return 0
	}
	removed := 0
	var kept []any
	for _, e := range entries {
		eo, ok := e.(*object)
		if !ok {
			kept = append(kept, e)
			continue
		}
		if cmd, ok := eo.get("command").(string); ok && ours(cmd) {
			removed++
			continue
		}
		if inner, ok := eo.get("hooks").([]any); ok {
			n := len(inner)
			inner = slices.DeleteFunc(inner, func(h any) bool {
				cmd, _ := path(h, "command").(string)
				return ours(cmd)
			})
			removed += n - len(inner)
			if len(inner) == 0 && n > 0 {
				continue
			}
			eo.set("hooks", inner)
		}
		kept = append(kept, e)
	}
	hooks.set(event, append([]any{}, kept...))
	pruneEmpty(hooks, event)
	pruneEmpty(root, "hooks")
	return removed
}

// ourHook matches hook commands gascoigne wrap -print writes for agent.
func ourHook(agent string) func(string) bool {
	return func(cmd string) bool {
		return strings.Contains(cmd, "gascoigne") && strings.HasSuffix(strings.TrimSpace(cmd), " hook "+agent)
	}
}

// removeMCP drops mcpServers.gascoigne when it runs gascoigne.
func removeMCP(root *object) bool {
	servers, ok := root.get("mcpServers").(*object)
	if !ok {
		return false
	}
	cmd, _ := path(servers, "gascoigne", "command").(string)
	if !strings.Contains(cmd, "gascoigne") {
		return false
	}
	servers.del("gascoigne")
	pruneEmpty(root, "mcpServers")
	return true
}
