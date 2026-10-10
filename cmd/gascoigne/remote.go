package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/extension/ledger"
	"github.com/lukaszb/gascoigne/internal/config"
	"github.com/lukaszb/gascoigne/internal/registry"
	"github.com/lukaszb/gascoigne/internal/service"
	"github.com/lukaszb/gascoigne/payload"
)

// optimizeRequest hands one CLI payload to the background server, which
// keeps config, components and the ledger warm between runs.
type optimizeRequest struct {
	// Build and Ledger must match the server's, else the CLI optimizes
	// itself: another build may optimize differently, another ledger is
	// someone else's (an isolated eval).
	Build  string `json:"build"`
	Ledger string `json:"ledger"`
	// Config is the effective YAML (defaults plus user file), Overrides the
	// run's -set/-only/-disable: the server builds the same pipelines.
	Config    string           `json:"config"`
	Overrides config.Overrides `json:"overrides"`
	Pipeline  string           `json:"pipeline"`
	Payload   *payload.Payload `json:"payload"`
}

// errUnavailable: the server can't take the request; optimize here instead.
var errUnavailable = errors.New("server unavailable")

// clientEnv are the GASCOIGNE_* variables the CLI itself turns into request
// fields (identity attrs, overrides, config) or reads. Any other one changes
// what components do in this process only, so the run stays local.
var clientEnv = []string{
	"GASCOIGNE_SESSION", "GASCOIGNE_AGENT", "GASCOIGNE_MODEL",
	"GASCOIGNE_SET", "GASCOIGNE_ONLY", "GASCOIGNE_DISABLE",
	"GASCOIGNE_CONFIG", "GASCOIGNE_SERVER", "GASCOIGNE_SERVER_ADDR",
}

// processEnv reports a GASCOIGNE_* variable set that is not in clientEnv.
func processEnv(env []string) bool {
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GASCOIGNE_") && v != "" && !slices.Contains(clientEnv, k) {
			return true
		}
	}
	return false
}

// remote sends payloads to the background server and falls back to a local
// pipeline when it can't serve them; nil when this run has to stay local.
func (c *common) remote(data []byte, cfg *config.Config, log *slog.Logger) service.Remote {
	if serverOff() || processEnv(os.Environ()) {
		return nil
	}
	path, ok := ledgerFile(cfg)
	if !ok {
		return nil
	}
	return func(ctx context.Context, pipeline string, p *payload.Payload) (*payload.Payload, error) {
		out, err := postOptimize(ctx, serverAddr(), optimizeRequest{
			Build: build(), Ledger: path, Config: string(data), Overrides: c.overrides(),
			Pipeline: pipeline, Payload: p,
		})
		if !errors.Is(err, errUnavailable) {
			return out, err
		}
		log.Debug("optimizing here", "err", err)
		return c.optimizeLocal(ctx, pipeline, p)
	}
}

// optimizeLocal builds the whole pipeline in this process for one payload;
// starting it also starts or replaces the server for the next run.
func (c *common) optimizeLocal(ctx context.Context, pipeline string, p *payload.Payload) (*payload.Payload, error) {
	local := *c
	local.useServer = false
	svc, err := local.start(ctx)
	if err != nil {
		return nil, err
	}
	defer svc.Shutdown(context.WithoutCancel(ctx))
	pl, ok := svc.Pipeline(pipeline)
	if !ok {
		return nil, fmt.Errorf("no pipeline %q", pipeline)
	}
	return pl.Optimize(ctx, p)
}

// ledgerFile is the db the config's ledger extension opens in this process.
func ledgerFile(cfg *config.Config) (string, bool) {
	for id, node := range cfg.Extensions {
		if pid, err := component.ParseID(id); err != nil || pid.Type != "ledger" {
			continue
		}
		lc, _ := ledger.NewFactory().DefaultConfig().(*ledger.Config)
		if config.DecodeNode(node, lc) != nil {
			return "", false
		}
		path, err := lc.File()
		return path, err == nil
	}
	return "", false
}

// remoteClient gives up fast when no server listens; optimizing a large
// payload may take a while once one does.
var remoteClient = &http.Client{
	Timeout:   30 * time.Second,
	Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 200 * time.Millisecond}).DialContext},
}

// postOptimize asks the server at addr to optimize. errUnavailable means
// it could not (none running, other build or ledger, bad config); other
// errors come from the pipeline itself.
func postOptimize(ctx context.Context, addr string, req optimizeRequest) (*payload.Payload, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/api/optimize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	r.Header.Set("Content-Type", "application/json")
	resp, err := remoteClient.Do(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUnavailable, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		var out payload.Payload
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, fmt.Errorf("%w: %v", errUnavailable, err)
		}
		return &out, nil
	case http.StatusUnprocessableEntity:
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.New(strings.TrimSpace(string(msg)))
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return nil, fmt.Errorf("%w: %s: %s", errUnavailable, resp.Status, strings.TrimSpace(string(msg)))
}

// warm keeps one started service per client config, so CLI runs skip
// building components and opening the ledger.
type warm struct {
	ctx    context.Context // lifetime of the services
	build  string
	ledger string
	mu     sync.Mutex // one payload at a time: components need not be concurrency-safe
	svcs   map[string]*service.Service
}

// maxWarm bounds services kept for distinct configs (-set runs each make one).
const maxWarm = 8

func newWarm(ctx context.Context, build, ledgerPath string) *warm {
	return &warm{ctx: ctx, build: build, ledger: ledgerPath, svcs: map[string]*service.Service{}}
}

func (w *warm) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var req optimizeRequest
	if err := json.NewDecoder(http.MaxBytesReader(rw, r.Body, 256<<20)).Decode(&req); err != nil || req.Payload == nil {
		http.Error(rw, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}
	if req.Build != w.build || req.Ledger != w.ledger {
		http.Error(rw, "other build or ledger", http.StatusConflict)
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	svc, err := w.service(req)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusConflict)
		return
	}
	pl, ok := svc.Pipeline(req.Pipeline)
	if !ok {
		http.Error(rw, fmt.Sprintf("no pipeline %q", req.Pipeline), http.StatusConflict)
		return
	}
	out, err := pl.Optimize(r.Context(), req.Payload)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	respond(rw, out, nil)
}

// service returns the started service for the request's config.
func (w *warm) service(req optimizeRequest) (*service.Service, error) {
	over, _ := json.Marshal(req.Overrides)
	sum := sha256.Sum256(append([]byte(req.Config+"\x00"), over...))
	key := hex.EncodeToString(sum[:])
	if svc, ok := w.svcs[key]; ok {
		return svc, nil
	}
	cfg, err := config.ParseWith([]byte(req.Config), req.Overrides)
	if err != nil {
		return nil, err
	}
	// Receivers and exporters belong to the CLI run: the server only optimizes.
	for name, pc := range cfg.Pipelines {
		pc.Receivers, pc.Exporters = nil, nil
		cfg.Pipelines[name] = pc
	}
	cfg.Receivers, cfg.Exporters = nil, nil
	svc, err := service.New(w.ctx, cfg, registry.Factories(), service.Options{})
	if err != nil {
		return nil, err
	}
	if err := svc.Start(w.ctx); err != nil {
		return nil, err
	}
	if l, ok := svc.Ledger().(*ledger.Ledger); !ok || l.Path() != w.ledger {
		svc.Shutdown(w.ctx)
		return nil, errors.New("config opens another ledger")
	}
	if len(w.svcs) >= maxWarm {
		w.shutdown()
	}
	w.svcs[key] = svc
	return svc, nil
}

func (w *warm) shutdown() {
	for key, svc := range w.svcs {
		svc.Shutdown(context.WithoutCancel(w.ctx))
		delete(w.svcs, key)
	}
}

// Close stops every warm service.
func (w *warm) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shutdown()
}
