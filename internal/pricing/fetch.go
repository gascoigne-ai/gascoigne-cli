package pricing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/httpget"
)

// Refresh runs every remote layer that is due (all of them with force), updating the cache. A
// failure keeps the layer's last good prices and is returned. Another process holding the lock
// is not an error: it is already doing the work.
func Refresh(ctx context.Context, cfg Config, force bool) error {
	names := cfg.remote()
	if len(names) == 0 {
		return nil
	}
	cached := readCache(cfg.StateDir)
	var todo []string
	for _, name := range names {
		if force || cfg.due(name, cached.Layers[name]) {
			todo = append(todo, name)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	release, ok := lock(cfg)
	if !ok {
		return nil
	}
	defer release()
	// Another process may have refreshed while this one waited for the lock.
	cached = readCache(cfg.StateDir)
	var errs []error
	for _, name := range todo {
		e := cached.Layers[name]
		if e == nil {
			e = &entry{}
			cached.Layers[name] = e
		}
		if !force && !cfg.due(name, e) {
			continue
		}
		e.Attempt = cfg.now()
		if err := cfg.run(ctx, name, e, force); err != nil {
			e.Error = err.Error()
			errs = append(errs, err)
		} else {
			e.Error, e.Fetched = "", cfg.now()
		}
	}
	if err := cached.write(cfg.StateDir); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// lock takes the refresh lock; a lock older than the longest run is stale.
func lock(cfg Config) (release func(), ok bool) {
	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		return nil, false
	}
	path := filepath.Join(cfg.StateDir, lockFile)
	for range 2 {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(path) }, true
		}
		info, statErr := os.Stat(path)
		if statErr != nil || time.Since(info.ModTime()) < 2*cfg.timeout()+time.Minute {
			return nil, false
		}
		os.Remove(path)
	}
	return nil, false
}

// run fetches or translates one layer into e.
func (c Config) run(ctx context.Context, name string, layer *entry, force bool) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	var input []byte
	switch name {
	case LayerJSON:
		stamp, err := fileStamp(c.File)
		if err != nil {
			return fmt.Errorf("%s prices: %w", name, err)
		}
		if input, err = readSheet(c.File); err != nil {
			return fmt.Errorf("%s prices: %w", name, err)
		}
		layer.Stamp = stamp
	case LayerAPI:
		body, notModified, err := c.fetch(ctx, layer, force)
		if err != nil {
			return fmt.Errorf("%s prices: %w", name, err)
		}
		if notModified {
			return nil
		}
		input = body
	}
	sheet := input
	if c.Translate != "" {
		var err error
		if sheet, err = c.translate(ctx, input); err != nil {
			return fmt.Errorf("%s prices: %w", name, err)
		}
	}
	if _, err := cost.ParsePrices(sheet, name); err != nil {
		return err
	}
	layer.Sheet = sheet
	return nil
}

// fetch GETs the URL, conditionally unless forced.
func (c Config) fetch(ctx context.Context, layer *entry, force bool) (body []byte, notModified bool, err error) {
	if err := checkURL(c.URL); err != nil {
		return nil, false, err
	}
	header := http.Header{}
	header.Set("Accept", "application/json")
	if c.HeaderEnv != "" {
		if name, value, ok := strings.Cut(os.Getenv(c.HeaderEnv), ":"); ok && strings.TrimSpace(name) != "" {
			header.Set(strings.TrimSpace(name), strings.TrimSpace(value))
		}
	}
	if !force && len(layer.Sheet) > 0 {
		if layer.ETag != "" {
			header.Set("If-None-Match", layer.ETag)
		}
		if layer.Modified != "" {
			header.Set("If-Modified-Since", layer.Modified)
		}
	}
	client := c.Client
	if client == nil {
		client = &http.Client{CheckRedirect: redirectPolicy}
	}
	response, err := httpget.Do(ctx, client, c.URL, header)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && len(layer.Sheet) > 0 {
		return nil, true, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%s: %s", c.URL, response.Status)
	}
	if body, err = httpget.Bytes(response, MaxBytes); err != nil {
		return nil, false, err
	}
	layer.ETag, layer.Modified = response.Header.Get("ETag"), response.Header.Get("Last-Modified")
	return body, false, nil
}

// MaxRedirects bounds the redirects a price URL may follow.
const MaxRedirects = 3

// checkURL accepts an http or https address with a host and no credentials in it.
func checkURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("url %q: want http or https", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("url %q: no host", raw)
	}
	if parsed.User != nil {
		return fmt.Errorf("url: credentials in the address are not accepted; use header_env for a token")
	}
	return nil
}

// redirectPolicy follows at most MaxRedirects redirects, each to an acceptable address, and never
// from https down to http.
func redirectPolicy(next *http.Request, via []*http.Request) error {
	if len(via) > MaxRedirects {
		return fmt.Errorf("more than %d redirects", MaxRedirects)
	}
	if err := checkURL(next.URL.String()); err != nil {
		return fmt.Errorf("redirect: %w", err)
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && next.URL.Scheme != "https" {
		return fmt.Errorf("redirect from https to http refused")
	}
	return nil
}

// limited is a writer that fails once more than MaxBytes arrive.
type limited struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (l *limited) Write(chunk []byte) (int, error) {
	if l.buf.Len()+len(chunk) > l.max {
		l.overflow = true
		return 0, errors.New("output too large")
	}
	return l.buf.Write(chunk)
}

// translate runs the command: input on stdin, the price sheet on stdout.
func (c Config) translate(ctx context.Context, input []byte) ([]byte, error) {
	argv, err := SplitArgs(c.Translate)
	if err != nil {
		return nil, err
	}
	var out, errOut limited
	out.max, errOut.max = MaxBytes, 4<<10
	program, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, fmt.Errorf("command %s: %w", argv[0], err)
	}
	command := exec.CommandContext(ctx, program, argv[1:]...)
	command.Stdin = bytes.NewReader(input)
	command.Stdout, command.Stderr = &out, &errOut
	command.WaitDelay = time.Second
	if err := command.Run(); err != nil {
		switch {
		case ctx.Err() != nil:
			return nil, fmt.Errorf("%s: timed out after %s", argv[0], c.timeout())
		case out.overflow:
			return nil, fmt.Errorf("%s: output over %d bytes", argv[0], MaxBytes)
		}
		if msg := strings.TrimSpace(errOut.buf.String()); msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", argv[0], err, msg)
		}
		return nil, fmt.Errorf("%s: %w", argv[0], err)
	}
	return out.buf.Bytes(), nil
}
