// Package mcpmock is a deterministic fake MCP server. Its tools return fake
// data in the shape real servers return (issue lists, code search hits, file
// reads, logs, directory trees, SQL rows, docs pages), so the proxy's savings
// on MCP results can be measured without a third-party server, network or
// credentials.
package mcpmock

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Tool is one fake tool: its definition and a generator for its result text.
type Tool struct {
	Name, Description string
	Text              func() string
}

// Tools is the catalog, in listing order. A new tool is a new row.
var Tools = []Tool{
	{"list_issues", "List the issues of a repository with their state, labels and authors.", issues},
	{"search_code", "Search the code of a repository; returns file:line: match hits.", codeHits},
	{"read_file", "Read a source file of a repository.", sourceFile},
	{"get_logs", "Fetch the last log lines of a service.", logs},
	{"list_directory", "List a directory tree of a repository.", tree},
	{"query", "Run a read-only SQL query; returns rows as JSON.", rows},
	{"get_page", "Fetch a documentation page as markdown.", page},
	{"get_status", "Report whether the service is up.", func() string { return `{"status":"ok","uptime_s":86400}` }},
}

// NewServer returns a server with every tool of Tools.
func NewServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "mock", Version: "1"}, nil)
	for _, t := range Tools {
		srv.AddTool(&mcp.Tool{Name: t.Name, Description: t.Description, InputSchema: map[string]any{"type": "object"}},
			func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: t.Text()}}}, nil
			})
	}
	return srv
}

// Call runs tool name through a real MCP session and returns its text, the
// path an agent's call takes (listing, protocol, content blocks).
func Call(ctx context.Context, name string) (string, error) {
	st, ct := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go NewServer().Run(ctx, st)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		return "", err
	}
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name})
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String(), nil
}

// rng is seeded per generator, so every run produces the same bytes.
func rng(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, seed)) }

func pick[T any](r *rand.Rand, xs ...T) T { return xs[r.IntN(len(xs))] }

func issues() string {
	r := rng(1)
	var b strings.Builder
	b.WriteString("[")
	for i := range 40 {
		if i > 0 {
			b.WriteString(",")
		}
		n := 1200 + i
		user := pick(r, "mara-k", "devon-p", "ines-r", "tomasz-w")
		fmt.Fprintf(&b, `{"id":%d,"number":%d,"title":%q,"state":%q,"locked":false,"user":{"login":%q,"id":%d,"type":"User","site_admin":false,"url":"https://api.example.com/users/%s","html_url":"https://example.com/%s","avatar_url":"https://avatars.example.com/u/%d?v=4"},"labels":[{"id":%d,"name":%q,"color":"d73a4a","default":true},{"id":%d,"name":%q,"color":"0075ca","default":false}],"comments":%d,"created_at":"2026-0%d-1%dT09:%02d:11Z","updated_at":"2026-0%d-2%dT14:%02d:40Z","url":"https://api.example.com/repos/acme/shop/issues/%d","html_url":"https://example.com/acme/shop/issues/%d","body":%q}`,
			9000000+n, n, pick(r, "Crash on empty cart", "Checkout total off by one cent", "Add retry to payment webhook", "Flaky TestReserve on CI", "Docs: explain inventory sync"),
			pick(r, "open", "open", "closed"), user, 100+r.IntN(900), user, user, 100+r.IntN(900),
			1+r.IntN(9), pick(r, "bug", "enhancement", "docs"), 20+r.IntN(9), pick(r, "needs-triage", "p1", "good first issue"), r.IntN(14),
			1+r.IntN(8), r.IntN(9), r.IntN(60), 1+r.IntN(8), r.IntN(9), r.IntN(60), n, n,
			"Steps to reproduce:\n1. Open the shop\n2. Add an item\n3. Remove it\nExpected: empty cart page. Actual: stack trace in the console.")
	}
	b.WriteString("]")
	return b.String()
}

func codeHits() string {
	r := rng(2)
	var b strings.Builder
	for range 70 {
		fmt.Fprintf(&b, "internal/%s/%s.go:%d: %s\n", pick(r, "orders", "stock", "price", "sync"), pick(r, "service", "store", "handler"), 1+r.IntN(400),
			pick(r, "func (s *Service) Reserve(ctx context.Context, sku string, n int) error {", "\treturn fmt.Errorf(\"reserve %s: %w\", sku, err)", "\tif n <= 0 {", "\tdefer s.mu.Unlock()"))
	}
	return b.String()
}

func sourceFile() string {
	var b strings.Builder
	b.WriteString("package stock\n\nimport (\n\t\"context\"\n\t\"fmt\"\n\t\"sync\"\n)\n\n")
	for i := range 14 {
		fmt.Fprintf(&b, "// Op%d adjusts the reserved count of a sku and reports the new level.\nfunc (s *Store) Op%d(ctx context.Context, sku string, n int) (int, error) {\n\ts.mu.Lock()\n\tdefer s.mu.Unlock()\n\tcur, ok := s.items[sku]\n\tif !ok {\n\t\treturn 0, fmt.Errorf(\"op%d %%s: unknown sku\", sku)\n\t}\n\tif cur.reserved+n > cur.total {\n\t\treturn cur.reserved, ErrShort\n\t}\n\tcur.reserved += n\n\treturn cur.reserved, nil\n}\n\n", i, i, i)
	}
	return b.String() + "var _ sync.Locker = (*sync.Mutex)(nil)\n"
}

func logs() string {
	r := rng(4)
	var b strings.Builder
	for i := range 300 {
		lvl, msg := "INFO", fmt.Sprintf("request served path=/api/items/%d status=200 ms=%d", r.IntN(50), 3+r.IntN(40))
		if i == 217 {
			lvl, msg = "ERROR", "payment webhook failed: dial tcp 10.0.3.7:443: connect: connection refused"
		}
		fmt.Fprintf(&b, "2026-10-29T10:%02d:%02d.%03dZ %s svc=orders %s\n", i/60, i%60, r.IntN(1000), lvl, msg)
	}
	return b.String()
}

func tree() string {
	r := rng(5)
	var b strings.Builder
	for _, d := range []string{"cmd", "internal/orders", "internal/stock", "internal/price", "internal/sync", "docs", "testdata"} {
		fmt.Fprintf(&b, "%s/\n", d)
		for range 8 {
			fmt.Fprintf(&b, "  %s%s  %d bytes\n", pick(r, "service", "store", "handler", "types", "util"), pick(r, ".go", "_test.go", ".md"), 200+r.IntN(9000))
		}
	}
	return b.String()
}

func rows() string {
	r := rng(6)
	var b strings.Builder
	b.WriteString(`{"columns":["id","sku","qty","status","updated_at"],"rows":[`)
	for i := range 60 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"sku":"SKU-%04d","qty":%d,"status":%q,"updated_at":"2026-10-%02dT08:00:00Z"}`, i+1, r.IntN(9999), r.IntN(500), pick(r, "in_stock", "reserved", "backorder"), 1+r.IntN(28))
	}
	return b.String() + "]}"
}

func page() string {
	var b strings.Builder
	b.WriteString("# Inventory sync\n\nThe sync job reconciles warehouse stock with the shop every five minutes.\n\n")
	for i := range 12 {
		fmt.Fprintf(&b, "## Step %d\n\nThe worker reads the delta feed, validates each record against the sku table and writes the result in one transaction. On failure the batch is retried with backoff and the incident is logged.\n\n- input: delta feed page %d\n- output: stock table rows\n- retry: 3 attempts\n\n", i+1, i+1)
	}
	return b.String()
}
