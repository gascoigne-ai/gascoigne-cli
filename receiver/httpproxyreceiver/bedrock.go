package httpproxyreceiver

import (
	"net/url"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// bedrockRegion is an AWS region name: the upstream host is built from it,
// so nothing else may reach the URL.
var bedrockRegion = lazyre.New(`^[a-z]{2}(-gov)?-[a-z]+-\d$`)

// bedrockAPI is Claude on Amazon Bedrock (InvokeModel): the body is the
// Anthropic Messages format. Responses stream as a binary event stream,
// which the usage meter does not read.
var bedrockAPI = api{rewrite: anthropic}

// bedrock splits a /bedrock/<region>/model/<id>/invoke… path, the base URL
// gascoigne gives Claude Code as ANTHROPIC_BEDROCK_BASE_URL, into the
// Bedrock Runtime upstream (tmpl with {region} filled in) and the path.
func bedrock(p, tmpl string) (upstream, rest string, ok bool) {
	tail, found := strings.CutPrefix(p, "/bedrock/")
	if !found {
		return "", "", false
	}
	region, rest, _ := strings.Cut(tail, "/")
	if !bedrockRegion.MatchString(region) {
		return "", "", false
	}
	return strings.ReplaceAll(tmpl, "{region}", region), "/" + rest, true
}

// bedrockModel is the model id in an InvokeModel path: /model/<id>/invoke.
func bedrockModel(p string) string {
	_, tail, ok := strings.Cut(p, "/model/")
	if !ok {
		return ""
	}
	id, _, _ := strings.Cut(tail, "/")
	if m, err := url.PathUnescape(id); err == nil {
		return m
	}
	return id
}
