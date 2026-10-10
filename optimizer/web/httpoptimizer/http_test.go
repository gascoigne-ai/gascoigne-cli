package httpoptimizer

import (
	"context"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func TestRedirectAndProblem(t *testing.T) {
	in := "* Trying 1.2.3.4:443...\n* TLSv1.3 (OUT), TLS handshake\n> GET /orders HTTP/1.1\n> Host: api\n> Accept: */*\n> \n" +
		"< HTTP/1.1 301 Moved\r\n< Location: /v2/orders\r\n< Server: nginx\r\n< \r\n" +
		"< HTTP/2 429\r\n< retry-after: 30\r\n< date: today\r\n< \r\n" + `{"title": "Too Many Requests", "status": 429}` + "\n"
	f := NewFactory()
	c, err := f.Create(context.Background(), component.Settings{}, f.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.HTTP}}}
	if err := c.(*Optimizer).Optimize(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := p.Blocks[0].Text
	for _, want := range []string{"> GET /orders HTTP/1.1\n> Host: api\nHTTP/1.1 301 Moved\nLocation: /v2/orders\nHTTP/2 429\nretry-after: 30\n", `"title":"Too Many Requests"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, noise := range []string{"TLS", "Accept", "nginx", "date"} {
		if strings.Contains(got, noise) {
			t.Errorf("kept %q:\n%s", noise, got)
		}
	}
}
