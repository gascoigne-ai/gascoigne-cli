package k8soptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

func TestDescribeNoiseRulesVerify(t *testing.T) {
	if err := describeNoise.Verify(); err != nil {
		t.Fatal(err)
	}
}

// The regexp describeNoise replaced, kept to pin the old behavior.
var oldDescribeNoise = lazyre.New(`^\s*(Container ID|Image ID|Controlled By|Priority|Service Account|QoS Class|Node-Selectors|SeccompProfile|IPs|IP Family Policy|IP Families|Session Affinity|Internal Traffic Policy|Topology Spread Constraints|Priority Class Name|Runtime Class Name|Annotations):|:\s+<none>\s*$|^\s*(TokenExpirationSeconds|ConfigMapName|ConfigMapOptional|DownwardAPI|Optional):|^\s+node\.kubernetes\.io/(not-ready|unreachable):NoExecute op=Exists for 300s`)

func TestDescribeNoiseSameAsRegexp(t *testing.T) {
	var examples []string
	for _, r := range describeNoise {
		examples = append(examples, r.Yes...)
		examples = append(examples, r.No...)
	}
	examples = append(examples, "    Container ID:  containerd://abc", "Priority:  0", "Priority Class Name:  x", "Priority: ", "PriorityX:", "    Limits:  <none>",
		"Node-Selectors:  <none>", "Annotations:  <none>", "a:\t<none>\t", "a: <none> x", "a:<none>", "  Optional:  false", "TokenExpirationSeconds:  3607", "Optional: x",
		"Tolerations:  node.kubernetes.io/not-ready:NoExecute op=Exists for 300s", "                 node.kubernetes.io/not-ready:NoExecute op=Exists for 300s",
		"node.kubernetes.io/unreachable:NoExecute op=Exists for 300s", "  node.kubernetes.io/unreachable:NoExecute op=Exists for 300s", "  node.kubernetes.io/unreachable:NoExecute op=Exists for 3000s",
		"  node.kubernetes.io/memory-pressure:NoSchedule op=Exists", "IPs:", "IP Families:  <none>", "ConfigMapName:  kube-root-ca.crt", "DownwardAPI:  true")
	sameAsOld(t, oldDescribeNoise.MatchString, describeNoise.Match, examples)
}
