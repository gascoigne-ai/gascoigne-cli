package httpproxyreceiver

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

// tally is the usage a response reports, field by field as the API names
// it. Streams report it across events (and some repeat running totals), so
// each field keeps its largest value.
type tally struct {
	model                    string
	input, cached, written   int
	written1h                int // of written, the hour-long cache writes
	output                   int
	inputHasCached, reported bool
}

// usageReader adds what one decoded response object (a whole body or one
// stream event) says about usage to t.
type usageReader func(chunk map[string]any, counts *tally)

// maxBody bounds a non-streamed body kept to read its usage.
const maxBody = 8 << 20

// meter wraps a response body: bytes pass through unchanged while usage
// events are read, and the usage is recorded once the body is closed.
func (r *Receiver) meter(resp *http.Response, read usageReader, agent string) io.ReadCloser {
	m := &metered{body: resp.Body, read: read, r: r, agent: agent,
		stream: strings.Contains(resp.Header.Get("Content-Type"), "event-stream")}
	switch resp.Header.Get("Content-Encoding") {
	case "", "identity":
	case "gzip":
		m.inflate()
	default:
		return resp.Body // no stdlib decoder (br): not metered
	}
	return m
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
	// gzip bodies: compressed bytes go to zw, a goroutine inflates them
	// into feed and closes done.
	zw   *io.PipeWriter
	done chan struct{}
}

func (m *metered) Read(chunk []byte) (int, error) {
	n, err := m.body.Read(chunk)
	if m.zw != nil {
		m.zw.Write(chunk[:n])
	} else {
		m.feed(chunk[:n])
	}
	return n, err
}

// inflate reads a copy of a gzip body; the copy is drained to the end even
// when it does not inflate, so writes never block the response.
func (m *metered) inflate() {
	pr, pw := io.Pipe()
	m.zw, m.done = pw, make(chan struct{})
	go func() {
		defer close(m.done)
		if zr, err := gzip.NewReader(pr); err == nil {
			buf := make([]byte, 32<<10)
			for {
				n, err := zr.Read(buf)
				m.feed(buf[:n])
				if err != nil {
					break
				}
			}
		}
		io.Copy(io.Discard, pr)
	}()
}

func (m *metered) feed(data []byte) {
	if !m.stream {
		if m.buf.Len()+len(data) <= maxBody {
			m.buf.Write(data)
		}
		return
	}
	m.buf.Write(data)
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
	var chunk map[string]any
	if json.Unmarshal(data, &chunk) == nil {
		m.read(chunk, &m.t)
	}
}

func (m *metered) Close() error {
	err := m.body.Close()
	m.once.Do(func() {
		if m.zw != nil {
			m.zw.Close()
			<-m.done
		}
		m.record()
	})
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
		Input: m.t.input, CacheRead: m.t.cached, CacheWrite: m.t.written, CacheWrite1h: min(m.t.written1h, m.t.written), Output: m.t.output}
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
func (m *metered) whole(data []byte) {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return
	}
	for _, o := range append(asList(v), v) {
		if chunk, ok := o.(map[string]any); ok {
			m.read(chunk, &m.t)
		}
	}
}

// anthropicUsage: message_start carries input and cache counts in
// message.usage, message_delta the output; plain responses one usage.
func anthropicUsage(chunk map[string]any, counts *tally) {
	msg := chunk
	if m := obj(chunk, "message"); m != nil {
		msg = m
	}
	counts.setModel(msg["model"])
	u := obj(msg, "usage")
	if u == nil {
		u = obj(chunk, "usage")
	}
	if u != nil {
		counts.add(u, "input_tokens", &counts.input)
		counts.add(u, "cache_read_input_tokens", &counts.cached)
		counts.add(u, "cache_creation_input_tokens", &counts.written)
		if c := obj(u, "cache_creation"); c != nil {
			counts.add(c, "ephemeral_1h_input_tokens", &counts.written1h)
		}
		counts.add(u, "output_tokens", &counts.output)
	}
}

// chatUsage: the usage chunk (stream_options.include_usage) or body;
// prompt_tokens include cached ones.
func chatUsage(chunk map[string]any, counts *tally) {
	counts.setModel(chunk["model"])
	u := obj(chunk, "usage")
	if u == nil {
		return
	}
	counts.inputHasCached = true
	counts.add(u, "prompt_tokens", &counts.input)
	counts.add(u, "completion_tokens", &counts.output)
	if d := obj(u, "prompt_tokens_details"); d != nil {
		counts.add(d, "cached_tokens", &counts.cached)
	}
}

// responsesUsage: response.completed carries response.usage; input_tokens
// include cached ones.
func responsesUsage(chunk map[string]any, counts *tally) {
	if r := obj(chunk, "response"); r != nil {
		chunk = r
	}
	counts.setModel(chunk["model"])
	u := obj(chunk, "usage")
	if u == nil {
		return
	}
	counts.inputHasCached = true
	counts.add(u, "input_tokens", &counts.input)
	counts.add(u, "output_tokens", &counts.output)
	if d := obj(u, "input_tokens_details"); d != nil {
		counts.add(d, "cached_tokens", &counts.cached)
	}
}

// geminiUsage: every chunk carries running usageMetadata; prompt tokens
// include cached ones, output is candidates plus thoughts.
func geminiUsage(chunk map[string]any, counts *tally) {
	counts.setModel(chunk["modelVersion"])
	u := obj(chunk, "usageMetadata")
	if u == nil {
		return
	}
	counts.inputHasCached = true
	counts.add(u, "promptTokenCount", &counts.input)
	counts.add(u, "cachedContentTokenCount", &counts.cached)
	out := 0
	for _, k := range []string{"candidatesTokenCount", "thoughtsTokenCount"} {
		if n, ok := u[k].(float64); ok {
			out += int(n)
		}
	}
	counts.output = max(counts.output, out)
}

// add keeps the larger of *dst and u[key], when u has it.
func (t *tally) add(usage map[string]any, key string, dst *int) {
	if n, ok := usage[key].(float64); ok {
		*dst = max(*dst, int(n))
		t.reported = true
	}
}

func (t *tally) setModel(model any) {
	if s, _ := model.(string); s != "" {
		t.model = s
	}
}
