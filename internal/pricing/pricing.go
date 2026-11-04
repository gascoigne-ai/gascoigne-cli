// Package pricing collects the prices the user supplies, from up to four places, and keeps the
// remote ones fresh without ever blocking a run:
//
//	static   the settings (Config.Static)
//	json     a price sheet file (Config.File)
//	api      a URL returning a price sheet (Config.URL), cached for the TTL
//	script   a user command printing a price sheet (Config.Translate), cached for the TTL
//
// The command may also translate: with a file or URL set, the file or response goes to its
// stdin and it prints the gascoigne price sheet (see cost.ParsePrices). On a model both give,
// static beats json beats api beats script, and all beat the built-in price (package cost).
package pricing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// Defaults and limits.
const (
	DefaultTTL     = 24 * time.Hour
	MinTTL         = time.Hour
	DefaultTimeout = 10 * time.Second
	// FailureBackoff is how long a failed remote layer waits before its next automatic try.
	FailureBackoff = time.Hour
	// MaxBytes caps a response and a script's output.
	MaxBytes = 1 << 20

	cacheFile = "prices-cache.json"
	lockFile  = "prices.lock"
)

// Layer names, also the Source of the prices they give.
const (
	LayerStatic = "static"
	LayerJSON   = "json"
	LayerAPI    = "api"
	LayerScript = "script"
)

// Config says where prices come from. The zero value gives none.
type Config struct {
	Static map[string]cost.PriceItem
	// File is a price sheet; with Translate it is the translator's stdin.
	File string
	// URL returns a price sheet (or, with Translate, the translator's stdin).
	URL string
	// HeaderEnv names an environment variable holding one "Name: value" request header, so a
	// token never sits in the settings.
	HeaderEnv string
	// Translate is a command line (see SplitArgs), run without a shell.
	Translate string
	// TTL is how long fetched and translated prices stay fresh; 0 means DefaultTTL, unless Manual.
	TTL time.Duration
	// Manual never refreshes by itself: only Refresh with force does.
	Manual bool
	// Timeout bounds one fetch or script run; 0 means DefaultTimeout.
	Timeout time.Duration
	// StateDir holds the cache and the lock.
	StateDir string

	// Client and Now are replaced by tests.
	Client *http.Client
	Now    func() time.Time
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Config) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return DefaultTTL
}

func (c Config) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return DefaultTimeout
}

// ParseTTL reads the interval setting: a Go duration, "" for the default, "0" or "manual" for
// no automatic runs. Less than MinTTL is raised to it, with a warning.
func ParseTTL(text string) (ttl time.Duration, manual bool, warning string, err error) {
	switch strings.TrimSpace(text) {
	case "":
		return DefaultTTL, false, "", nil
	case "0", "manual":
		return 0, true, "", nil
	}
	ttl, err = time.ParseDuration(strings.TrimSpace(text))
	if err != nil {
		return 0, false, "", fmt.Errorf("prices ttl %q: want a duration such as 12h, or manual", text)
	}
	if ttl < 0 {
		return 0, false, "", fmt.Errorf("prices ttl %q must not be negative", text)
	}
	if ttl == 0 {
		return 0, true, "", nil
	}
	if ttl < MinTTL {
		return MinTTL, false, fmt.Sprintf("prices ttl %s is below %s: using %s", text, MinTTL, MinTTL), nil
	}
	return ttl, false, "", nil
}

// Layer is the state of one source, for display.
type Layer struct {
	Name    string
	Models  int
	Fetched time.Time // when remote prices were last good; zero for static and a plain file
	Next    time.Time // when an automatic refresh is due; zero if none
	Error   string    // the last failure, cleared by the next good run
}

// Result is what Load found.
type Result struct {
	Overrides []cost.Override
	Layers    []Layer
	// Stale says a remote layer is due for Refresh.
	Stale bool
}

type entry struct {
	Sheet    json.RawMessage `json:"sheet,omitempty"`
	Stamp    string          `json:"stamp,omitempty"` // json layer: the file's size and mtime the sheet was made from
	Fetched  time.Time       `json:"fetched,omitzero"`
	Attempt  time.Time       `json:"attempt,omitzero"`
	Error    string          `json:"error,omitempty"`
	ETag     string          `json:"etag,omitempty"`
	Modified string          `json:"modified,omitempty"`
}

type cache struct {
	Layers map[string]*entry `json:"layers"`
}

func readCache(dir string) cache {
	c := cache{Layers: map[string]*entry{}}
	if data, err := os.ReadFile(filepath.Join(dir, cacheFile)); err == nil {
		json.Unmarshal(data, &c) // unreadable: nothing cached
	}
	if c.Layers == nil {
		c.Layers = map[string]*entry{}
	}
	return c
}

func (c cache) write(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(c, "", "  ")
	tmp := filepath.Join(dir, cacheFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, cacheFile))
}

// remote reports which cached layers the config uses.
func (c Config) remote() []string {
	var names []string
	if c.File != "" && c.Translate != "" {
		names = append(names, LayerJSON)
	}
	if c.URL != "" {
		names = append(names, LayerAPI)
	}
	if c.Translate != "" && c.File == "" && c.URL == "" {
		names = append(names, LayerScript)
	}
	return names
}

// fileStamp identifies a version of the file.
func fileStamp(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()), nil
}

// due reports whether a cached layer should be refreshed now by itself.
func (c Config) due(name string, e *entry) bool {
	if name == LayerJSON {
		// A changed file is news even in manual mode.
		stamp, err := fileStamp(c.File)
		if err == nil && (e == nil || e.Stamp != stamp) {
			return e == nil || e.Error == "" || c.now().Sub(e.Attempt) >= FailureBackoff
		}
	}
	if c.Manual {
		return false
	}
	if e == nil || e.Fetched.IsZero() && e.Attempt.IsZero() {
		return true
	}
	if e.Error != "" && c.now().Sub(e.Attempt) < FailureBackoff {
		return false
	}
	return c.now().Sub(e.Fetched) >= c.ttl()
}

// Load builds the user's prices from the settings and the cache. It reads files and never the
// network or the script; Stale says a Refresh is due.
func Load(cfg Config) Result {
	var result Result
	cached := readCache(cfg.StateDir)
	var lists [][]cost.Override
	add := func(layer Layer, list []cost.Override, err error) {
		if err != nil {
			layer.Error = err.Error()
		} else {
			layer.Models = len(list)
			lists = append(lists, list)
		}
		result.Layers = append(result.Layers, layer)
	}

	if len(cfg.Static) > 0 {
		sheet, err := cost.Sheet(cfg.Static)
		var list []cost.Override
		if err == nil {
			list, err = cost.ParsePrices(sheet, LayerStatic)
		}
		add(Layer{Name: LayerStatic}, list, err)
	}
	if cfg.File != "" {
		layer := Layer{Name: LayerJSON}
		var list []cost.Override
		var err error
		if cfg.Translate == "" {
			var data []byte
			if data, err = os.ReadFile(cfg.File); err == nil {
				list, err = cost.ParsePrices(data, LayerJSON)
			}
		} else {
			e := cached.Layers[LayerJSON]
			list, err = fromCache(e, LayerJSON, &layer, cfg)
			if cfg.due(LayerJSON, e) {
				result.Stale = true
			}
		}
		add(layer, list, err)
	}
	for _, name := range []string{LayerAPI, LayerScript} {
		if !slices.Contains(cfg.remote(), name) {
			continue
		}
		layer := Layer{Name: name}
		e := cached.Layers[name]
		list, err := fromCache(e, name, &layer, cfg)
		if cfg.due(name, e) {
			result.Stale = true
		}
		add(layer, list, err)
	}

	// The first list that names a model wins.
	seen := map[string]bool{}
	for _, list := range lists {
		for _, o := range list {
			if !seen[o.Model.ID] {
				seen[o.Model.ID] = true
				result.Overrides = append(result.Overrides, o)
			}
		}
	}
	return result
}

// fromCache turns a layer's cached sheet into prices and fills its status. No sheet yet is not
// an error unless the last run failed.
func fromCache(e *entry, name string, layer *Layer, cfg Config) ([]cost.Override, error) {
	if e == nil {
		return nil, nil
	}
	layer.Fetched = e.Fetched
	if !cfg.Manual && !e.Fetched.IsZero() {
		layer.Next = e.Fetched.Add(cfg.ttl())
	}
	var err error
	if e.Error != "" {
		err = errors.New(e.Error)
	}
	if len(e.Sheet) == 0 {
		return nil, err
	}
	list, perr := cost.ParsePrices(e.Sheet, name)
	if perr != nil {
		return nil, perr
	}
	if err != nil { // last good prices stay in force; the status shows why they are old
		layer.Error = err.Error()
		layer.Models = len(list)
		return list, nil
	}
	return list, nil
}

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
func (c Config) run(ctx context.Context, name string, e *entry, force bool) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	var input []byte
	switch name {
	case LayerJSON:
		stamp, err := fileStamp(c.File)
		if err != nil {
			return fmt.Errorf("%s prices: %w", name, err)
		}
		if input, err = os.ReadFile(c.File); err != nil {
			return fmt.Errorf("%s prices: %w", name, err)
		}
		e.Stamp = stamp
	case LayerAPI:
		body, notModified, err := c.fetch(ctx, e, force)
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
	e.Sheet = sheet
	return nil
}

// fetch GETs the URL, conditionally unless forced.
func (c Config) fetch(ctx context.Context, e *entry, force bool) (body []byte, notModified bool, err error) {
	if !strings.HasPrefix(c.URL, "https://") && !strings.HasPrefix(c.URL, "http://") {
		return nil, false, fmt.Errorf("url %q: want http or https", c.URL)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("Accept", "application/json")
	if c.HeaderEnv != "" {
		if name, value, ok := strings.Cut(os.Getenv(c.HeaderEnv), ":"); ok && strings.TrimSpace(name) != "" {
			request.Header.Set(strings.TrimSpace(name), strings.TrimSpace(value))
		}
	}
	if !force && len(e.Sheet) > 0 {
		if e.ETag != "" {
			request.Header.Set("If-None-Match", e.ETag)
		}
		if e.Modified != "" {
			request.Header.Set("If-Modified-Since", e.Modified)
		}
	}
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified && len(e.Sheet) > 0 {
		return nil, true, nil
	}
	if response.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("%s: %s", c.URL, response.Status)
	}
	body, err = io.ReadAll(io.LimitReader(response.Body, MaxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(body) > MaxBytes {
		return nil, false, fmt.Errorf("response over %d bytes", MaxBytes)
	}
	e.ETag, e.Modified = response.Header.Get("ETag"), response.Header.Get("Last-Modified")
	return body, false, nil
}

// limited is a writer that fails once more than MaxBytes arrive.
type limited struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (l *limited) Write(p []byte) (int, error) {
	if l.buf.Len()+len(p) > l.max {
		l.overflow = true
		return 0, errors.New("output too large")
	}
	return l.buf.Write(p)
}

// translate runs the command: input on stdin, the price sheet on stdout.
func (c Config) translate(ctx context.Context, input []byte) ([]byte, error) {
	argv, err := SplitArgs(c.Translate)
	if err != nil {
		return nil, err
	}
	var out, errOut limited
	out.max, errOut.max = MaxBytes, 4<<10
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
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

// SplitArgs splits a command line into arguments: spaces separate, single and double quotes
// group, a backslash escapes the next character. There is no shell: no expansion, pipes or
// redirects.
func SplitArgs(line string) ([]string, error) {
	var args []string
	var current strings.Builder
	started := false
	var quote rune
	escaped := false
	for _, r := range line {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped, started = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				current.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, started = r, true
		case r == ' ' || r == '\t':
			if started {
				args, started = append(args, current.String()), false
				current.Reset()
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("command %s: unterminated quote", strconv.Quote(line))
	}
	if started {
		args = append(args, current.String())
	}
	if len(args) == 0 {
		return nil, errors.New("command is empty")
	}
	return args, nil
}
