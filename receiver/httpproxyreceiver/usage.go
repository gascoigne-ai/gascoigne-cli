package httpproxyreceiver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lukaszb/gascoigne/component"
)

// tally is the usage a response reports, field by field as the API names
// it. Streams report it across events (and some repeat running totals), so
// each field keeps its largest value.
type tally struct {
	model                    string
	input, cached, written   int
	output                   int
	inputHasCached, reported bool
}

// usageReader adds what one decoded response object (a whole body or one
// stream event) says about usage to t.
type usageReader func(obj map[string]any, t *tally)

// maxBody bounds a non-streamed body kept to read its usage.
const maxBody = 8 << 20

// meter wraps a response body: bytes pass through unchanged while usage
// events are read, and the usage is recorded once the body is closed.
func (r *Receiver) meter(resp *http.Response, read usageReader, agent string) io.ReadCloser {
	return &metered{body: resp.Body, read: read, r: r, agent: agent,
		stream: strings.Contains(resp.Header.Get("Content-Type"), "event-stream")}
}

type metered struct {
	body   io.ReadCloser
	read   usageReader
	r      *Receiver
	agent  string
	stream bool
	buf    bytes.Buffer // stream: the unfinished line; else the body so far
	t      tally
	once   sync.Once
}

func (m *metered) Read(p []byte) (int, error) {
	n, err := m.body.Read(p)
	m.feed(p[:n])
	return n, err
}

func (m *metered) feed(b []byte) {
	if !m.stream {
		if m.buf.Len()+len(b) <= maxBody {
			m.buf.Write(b)
		}
		return
	}
	m.buf.Write(b)
	for {
		line, err := m.buf.ReadBytes('\n')
		if err != nil {
			m.buf.Write(line) // incomplete: wait for the rest
			return
		}
		m.event(line)
	}
}

// event reads one stream line; only data lines mentioning usage are decoded.
func (m *metered) event(line []byte) {
	data, ok := bytes.CutPrefix(bytes.TrimSpace(line), []byte("data:"))
	if !ok || !bytes.Contains(data, []byte("sage")) { // usage, usageMetadata
		return
	}
	var obj map[string]any
	if json.Unmarshal(data, &obj) == nil {
		m.read(obj, &m.t)
	}
}

func (m *metered) Close() error {
	err := m.body.Close()
	m.once.Do(m.record)
	return err
}

func (m *metered) record() {
	if m.stream {
		m.event(m.buf.Bytes())
	} else {
		m.whole(m.buf.Bytes())
	}
	if !m.t.reported {
		return
	}
	u := component.Usage{TS: time.Now(), Agent: m.agent, Model: m.t.model,
		Input: m.t.input, CacheRead: m.t.cached, CacheWrite: m.t.written, Output: m.t.output}
	if m.t.inputHasCached {
		u.Input = max(0, m.t.input-m.t.cached)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.r.usage.RecordUsage(ctx, u); err != nil {
		m.r.log.Warn("record usage", "err", err)
	}
}

// whole reads a whole JSON response: one object, or an array of stream
// chunks (Gemini without alt=sse).
func (m *metered) whole(b []byte) {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return
	}
	for _, o := range append(asList(v), v) {
		if obj, ok := o.(map[string]any); ok {
			m.read(obj, &m.t)
		}
	}
}

// anthropicUsage: message_start carries input and cache counts in
// message.usage, message_delta the output; plain responses one usage.
func anthropicUsage(obj map[string]any, t *tally) {
	msg := obj
	if m, ok := obj["message"].(map[string]any); ok {
		msg = m
	}
	t.setModel(msg["model"])
	u, ok := msg["usage"].(map[string]any)
	if !ok {
		u, ok = obj["usage"].(map[string]any)
	}
	if ok {
		t.add(u, "input_tokens", &t.input)
		t.add(u, "cache_read_input_tokens", &t.cached)
		t.add(u, "cache_creation_input_tokens", &t.written)
		t.add(u, "output_tokens", &t.output)
	}
}

// chatUsage: the usage chunk (stream_options.include_usage) or body;
// prompt_tokens include cached ones.
func chatUsage(obj map[string]any, t *tally) {
	t.setModel(obj["model"])
	u, ok := obj["usage"].(map[string]any)
	if !ok {
		return
	}
	t.inputHasCached = true
	t.add(u, "prompt_tokens", &t.input)
	t.add(u, "completion_tokens", &t.output)
	if d, ok := u["prompt_tokens_details"].(map[string]any); ok {
		t.add(d, "cached_tokens", &t.cached)
	}
}

// responsesUsage: response.completed carries response.usage; input_tokens
// include cached ones.
func responsesUsage(obj map[string]any, t *tally) {
	if r, ok := obj["response"].(map[string]any); ok {
		obj = r
	}
	t.setModel(obj["model"])
	u, ok := obj["usage"].(map[string]any)
	if !ok {
		return
	}
	t.inputHasCached = true
	t.add(u, "input_tokens", &t.input)
	t.add(u, "output_tokens", &t.output)
	if d, ok := u["input_tokens_details"].(map[string]any); ok {
		t.add(d, "cached_tokens", &t.cached)
	}
}

// geminiUsage: every chunk carries running usageMetadata; prompt tokens
// include cached ones, output is candidates plus thoughts.
func geminiUsage(obj map[string]any, t *tally) {
	t.setModel(obj["modelVersion"])
	u, ok := obj["usageMetadata"].(map[string]any)
	if !ok {
		return
	}
	t.inputHasCached = true
	t.add(u, "promptTokenCount", &t.input)
	t.add(u, "cachedContentTokenCount", &t.cached)
	out := 0
	for _, k := range []string{"candidatesTokenCount", "thoughtsTokenCount"} {
		if n, ok := u[k].(float64); ok {
			out += int(n)
		}
	}
	t.output = max(t.output, out)
}

// add keeps the larger of *dst and u[key], when u has it.
func (t *tally) add(u map[string]any, key string, dst *int) {
	if n, ok := u[key].(float64); ok {
		*dst = max(*dst, int(n))
		t.reported = true
	}
}

func (t *tally) setModel(v any) {
	if s, _ := v.(string); s != "" {
		t.model = s
	}
}
