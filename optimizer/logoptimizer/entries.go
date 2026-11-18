package logoptimizer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Field candidates of cloud log events, most specific first.
var (
	timeKeys = []string{"timestamp", "eventTimestamp", "time", "@timestamp", "TimeGenerated", "receiveTimestamp"}
	sevKeys  = []string{"severity", "level", "Level", "status.value"}
	msgKeys  = []string{"message", "msg", "textPayload", "jsonPayload.message", "jsonPayload.msg", "protoPayload.status.message",
		"protoPayload.methodName", "operationName.localizedValue", "operationName.value", "Message"}
	// Lists of events inside a response object.
	listKeys = []string{"events", "logEvents", "entries", "value", "tables"}
)

// entries turns structured log events (aws logs JSON, gcloud logging YAML or
// JSON, az monitor JSON) into "time severity message" lines.
func entries(text string) ([]string, bool) {
	items, flat, ok := events(strings.TrimSpace(text))
	if !ok || len(items) == 0 {
		return nil, false
	}
	var out []string
	messages := 0
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, false
		}
		msg := pick(m, msgKeys)
		if msg != "" {
			messages++
		}
		parts := []string{when(pick(m, timeKeys)), strings.ToUpper(pick(m, sevKeys)), request(m), msg}
		if flat {
			parts = append(parts, fields(m))
		} else if msg == "" && parts[2] == "" {
			b, _ := json.Marshal(m)
			parts = append(parts, string(b))
		}
		out = append(out, strings.Join(slices.DeleteFunc(parts, func(part string) bool { return part == "" }), " "))
	}
	return out, 2*messages >= len(items)
}

// fields renders the other scalar fields of an app log line as k=v.
func fields(entry map[string]any) string {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(entry)) {
		if slices.Contains(timeKeys, k) || slices.Contains(sevKeys, k) || slices.Contains(msgKeys, k) {
			continue
		}
		if v := scalar(entry[k]); v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, " ")
}

// events decodes a JSON array, a JSON object holding one, JSON lines (flat
// app logs) or YAML documents.
func events(text string) (items []any, flat, ok bool) {
	if !strings.HasPrefix(text, "{") && !strings.HasPrefix(text, "[") {
		docs, ok := yamlDocs(text)
		if !ok {
			return nil, false, false
		}
		return unwrapDocs(docs)
	}
	var v any
	if json.Unmarshal([]byte(text), &v) != nil {
		lines, ok := jsonLines(text)
		return lines, true, ok
	}
	return unwrapDocs([]any{v})
}

// yamlDocs decodes every YAML document of text.
func yamlDocs(text string) (docs []any, ok bool) {
	dec := yaml.NewDecoder(strings.NewReader(text))
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return docs, true
		}
		if err != nil {
			return nil, false
		}
		docs = append(docs, v)
	}
}

// unwrapDocs returns the entries inside a lone array or object holding one
// under a listKeys key; otherwise the documents themselves.
func unwrapDocs(docs []any) (items []any, flat, ok bool) {
	if len(docs) == 1 {
		if list, ok := entryList(docs[0]); ok {
			return list, false, true
		}
	}
	return docs, len(docs) == 1, true // one JSON line or YAML entries
}

// entryList is the array a document is, or the one it holds under a listKeys key.
func entryList(doc any) ([]any, bool) {
	switch v := doc.(type) {
	case []any:
		return v, true
	case map[string]any:
		for _, k := range listKeys {
			if list, ok := v[k].([]any); ok {
				return list, true
			}
		}
	}
	return nil, false
}

func jsonLines(text string) ([]any, bool) {
	var out []any
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		var v map[string]any
		if json.Unmarshal([]byte(l), &v) != nil {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

// pick returns the first present field of keys ("a.b" walks into objects).
func pick(entry map[string]any, keys []string) string {
	for _, k := range keys {
		var v any = entry
		for _, part := range strings.Split(k, ".") {
			obj, ok := v.(map[string]any)
			if !ok {
				v = nil
				break
			}
			v = obj[part]
		}
		if s := scalar(v); s != "" {
			return s
		}
	}
	return ""
}

// scalar renders a decoded value, or "" for nil, empty strings and containers.
func scalar(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimRight(v, "\n")
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64) // epoch ms, not 1.7e+12
	case int, bool, time.Time:
		return fmt.Sprint(v)
	}
	return ""
}

// when formats epoch milliseconds (aws) and keeps timestamps as they are.
func when(stamp string) string {
	var ms int64
	if _, err := fmt.Sscanf(stamp, "%d", &ms); err == nil && fmt.Sprint(ms) == stamp && ms > 1e11 {
		return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
	}
	return stamp
}

// request is the HTTP request of a load balancer or Cloud Run entry.
func request(entry map[string]any) string {
	r, ok := entry["httpRequest"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s %s", pick(r, []string{"requestMethod"}), pick(r, []string{"requestUrl"}), pick(r, []string{"status"})))
}
