package externaloptimizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// oneshot starts the child per block. text and json differ only in how the
// block goes in and the answer comes out.
type oneshot struct {
	cfg *Config
	log *slog.Logger
	// encode returns stdin and extra env; decode turns stdout into a Response.
	encode func(Request) ([]byte, []string, error)
	decode func(out []byte, req Request) (Response, error)
}

func newTextCodec(cfg *Config, log *slog.Logger) codec {
	return &oneshot{cfg: cfg, log: log, encode: encodeText, decode: decodeText}
}

func newJSONCodec(cfg *Config, log *slog.Logger) codec {
	return &oneshot{cfg: cfg, log: log, encode: encodeJSON, decode: decodeJSON}
}

func (o *oneshot) close() {}

func (o *oneshot) call(ctx context.Context, req Request) (Response, error) {
	stdin, env, err := o.encode(req)
	if err != nil {
		return Response{}, err
	}
	out, stderr, err := o.cfg.runOnce(ctx, stdin, env)
	if stderr != "" {
		o.log.Debug("external stderr", "stderr", stderr)
	}
	if err != nil {
		return Response{}, err
	}
	return o.decode(out, req)
}

func encodeText(req Request) ([]byte, []string, error) {
	env := append(attrEnv(req.Attrs),
		"GASCOIGNE_BLOCK_TYPE="+req.Block.Type,
		"GASCOIGNE_BLOCK_KIND="+req.Block.Kind)
	return []byte(req.Block.Text), env, nil
}

// decodeText takes stdout as the new text; the exit code was checked already.
func decodeText(out []byte, _ Request) (Response, error) {
	text := string(out)
	return Response{Version: Version, Text: &text}, nil
}

func encodeJSON(req Request) ([]byte, []string, error) {
	req.Version = Version
	data, err := json.Marshal(req)
	return data, nil, err
}

func decodeJSON(out []byte, _ Request) (Response, error) {
	var r Response
	dec := json.NewDecoder(bytes.NewReader(out))
	if err := dec.Decode(&r); err != nil {
		return Response{}, fmt.Errorf("bad json response: %w", err)
	}
	return r, nil
}
