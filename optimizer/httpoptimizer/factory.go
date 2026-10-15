package httpoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
)

const (
	typ    = "http"
	family = "web"
)

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config {
			return &Config{JSON: *jsonx.Default(), Links: true,
				DropHeaders: []string{"date", "server", "connection", "keep-alive", "vary", "via", "alt-svc", "nel", "report-to",
					"strict-transport-security", "x-content-type-options", "x-frame-options", "x-xss-protection", "referrer-policy",
					"permissions-policy", "content-security-policy", "cf-ray", "cf-cache-status", "x-cache", "x-cache-hits", "x-served-by",
					"x-timer", "age", "expires", "pragma", "accept-ranges", "x-amz-cf-id", "x-amz-cf-pop", "x-amz-id-2", "server-timing",
					"x-github-request-id", "x-fastly-request-id", "cross-origin-opener-policy", "cross-origin-resource-policy",
					"cross-origin-embedder-policy", "origin-agent-cluster", "x-powered-by", "x-envoy-upstream-service-time", "transfer-encoding",
					"content-length", "etag", "last-modified", "access-control-allow-origin", "access-control-expose-headers",
					"access-control-allow-credentials", "x-github-media-type", "x-github-api-version-selected", "x-github-edge-region",
					"x-cloud-trace-context", "x-ratelimit-resource"}}
		},
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		},
		component.WithFamily(family))
}
