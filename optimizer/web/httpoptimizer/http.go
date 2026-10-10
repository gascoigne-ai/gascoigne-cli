// Package httpoptimizer is the web family engine for HTTP responses with
// headers (curl -i, -I, -v; httpie -v): connection and TLS chatter and
// transport headers go, status lines and meaningful headers stay, and the
// body is compacted by its type: JSON crushed, HTML turned into markdown.
package httpoptimizer

import (
	"context"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/htmlx"
	"github.com/lukaszb/gascoigne/compress/jsonx"
	"github.com/lukaszb/gascoigne/compress/rows"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := rows.Terms(p.Attrs[payload.AttrQuery])
	return p.Rewrite(func(b payload.Block) (string, error) {
		head, body := o.split(compress.Lines(compress.StripANSI(b.Text)))
		return compress.Join(head) + o.body(body, query), nil
	}, detect.HTTP)
}

var (
	status = lazyre.New(`^HTTP/[\d.]+ \d{3}`)
	header = lazyre.New(`^([\w-]+):\s*(.*)$`)
	// curl -v lines worth keeping: where it connected and what failed.
	verboseKeep = lazyre.New(`^\* (Connected to|Established connection to|Could not resolve|Failed|Recv failure|SSL certificate problem|Connection refused|Operation timed out|Closing connection|Issue another request|.*error)`)
)

// split separates headers (of every response, redirects included) from the
// final body, keeping status lines, the request line and useful headers.
func (o *Optimizer) split(lines []string) (head, body []string) {
	inHeaders := false
	for i, l := range lines {
		raw := strings.TrimRight(l, "\r")
		switch {
		case strings.HasPrefix(raw, "* "): // curl -v info
			if verboseKeep.MatchString(raw) {
				head = append(head, raw)
			}
			continue
		case strings.HasPrefix(raw, "{ ") || strings.HasPrefix(raw, "} ") || raw == "<" || raw == ">":
			continue
		case strings.HasPrefix(raw, "> "): // request
			if r := strings.TrimSpace(raw[2:]); r != "" && (!header.MatchString(r) || strings.HasPrefix(strings.ToLower(r), "host:")) {
				head = append(head, raw)
			}
			continue
		case strings.HasPrefix(raw, "< "):
			raw = raw[2:]
		}
		if status.MatchString(raw) {
			inHeaders = true
			head = append(head, raw)
			continue
		}
		if !inHeaders {
			return head, lines[i:]
		}
		if raw == "" {
			inHeaders = false
			continue
		}
		if m := header.FindStringSubmatch(raw); m != nil && !slices.Contains(o.cfg.DropHeaders, strings.ToLower(m[1])) {
			head = append(head, raw)
		}
	}
	return head, nil
}

// body compacts the payload by its detected type.
func (o *Optimizer) body(lines []string, query []string) string {
	text := compress.Join(lines)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	switch detect.Sniff(text) {
	case detect.JSON, detect.NDJSON:
		if out, err := jsonx.Compress(text, detect.Sniff(text) == detect.NDJSON, &o.cfg.JSON, query); err == nil {
			return "\n" + out
		}
	case detect.HTML:
		if out, err := htmlx.Markdown(text, o.cfg.Links); err == nil {
			return "\n" + out
		}
	}
	return "\n" + text
}
