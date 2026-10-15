package cloudoptimizer

import (
	"maps"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/yamlx"
)

// dropKeys carry nothing for a reader: pagination, request metadata,
// concurrency tokens, API self references.
var dropKeys = map[string]bool{
	"ResponseMetadata": true, "NextToken": true, "nextPageToken": true, "nextLink": true, "@odata.nextLink": true,
	"etag": true, "ETag": true, "fingerprint": true, "labelFingerprint": true, "metadataFingerprint": true,
	"selfLink": true, "selfLinkWithId": true, "systemData": true, "RequestId": true,
}

// prune removes noise keys from decoded JSON, recursively.
func prune(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			// Succeeded is the expected state; gcp kinds ("compute#instance") repeat the type.
			if dropKeys[k] || k == "provisioningState" && x == "Succeeded" || k == "kind" && isGCPKind(x) {
				delete(v, k)
				continue
			}
			v[k] = prune(x)
		}
	case []any:
		for i, x := range v {
			v[i] = prune(x)
		}
	}
	return v
}

func isGCPKind(v any) bool {
	s, ok := v.(string)
	return ok && strings.Contains(s, "#") // compute#instance, sql#instance
}

// yamlRules: the same for YAML output (gcloud describe).
var yamlRules = func() []yamlx.Rule {
	var r []yamlx.Rule
	for _, k := range slices.Sorted(maps.Keys(dropKeys)) {
		r = append(r, yamlx.Rule{Key: k})
	}
	// Cloud Run (knative) bookkeeping written by the control plane and gcloud.
	for _, k := range []string{"uid", "resourceVersion", "generation", "observedGeneration",
		"run.googleapis.com/client-name", "run.googleapis.com/client-version", "run.googleapis.com/operation-id",
		"client.knative.dev/nonce", "serving.knative.dev/creator", "serving.knative.dev/lastModifier"} {
		r = append(r, yamlx.Rule{Key: k})
	}
	return append(r, yamlx.Rule{Key: "provisioningState", Equals: "Succeeded"})
}()
