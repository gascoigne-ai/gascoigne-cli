package k8soptimizer

import "github.com/gascoigne-ai/gascoigne-cli/internal/compress/yamlx"

// rules drop what Kubernetes adds or defaults: bookkeeping, server-side
// defaults, the service account token volume and default tolerations.
var rules = []yamlx.Rule{
	{Key: "managedFields"},
	{Key: "kubectl.kubernetes.io/last-applied-configuration"},
	{Key: "deployment.kubernetes.io/revision"},
	{Key: "uid", Parent: "metadata"},
	{Key: "resourceVersion"},
	{Key: "generation", Parent: "metadata"},
	{Key: "selfLink"},
	{Key: "uid", Parent: "ownerReferences"},
	{Key: "blockOwnerDeletion"},
	{Key: "containerID"},
	{Key: "imageID"},
	{Key: "lastProbeTime"},
	{Key: "terminationMessagePath", Equals: "/dev/termination-log"},
	{Key: "terminationMessagePolicy", Equals: "File"},
	{Key: "dnsPolicy", Equals: "ClusterFirst"},
	{Key: "restartPolicy", Equals: "Always"},
	{Key: "schedulerName", Equals: "default-scheduler"},
	{Key: "enableServiceLinks", Equals: "true"},
	{Key: "preemptionPolicy", Equals: "PreemptLowerPriority"},
	{Key: "priority", Equals: "0"},
	{Key: "terminationGracePeriodSeconds", Equals: "30"},
	{Key: "progressDeadlineSeconds", Equals: "600"},
	{Key: "revisionHistoryLimit", Equals: "10"},
	{Key: "protocol", Equals: "TCP"},
	{Key: "sessionAffinity", Equals: "None"},
	{Key: "ipFamilyPolicy", Equals: "SingleStack"},
	{Key: "internalTrafficPolicy", Equals: "Cluster"},
	{Key: "serviceAccount"}, // deprecated duplicate of serviceAccountName
	{Key: "volumes", ItemField: "name", ItemPrefix: "kube-api-access-"},
	{Key: "volumeMounts", ItemField: "name", ItemPrefix: "kube-api-access-"},
	{Key: "tolerations", ItemMatch: map[string]string{"key": "node.kubernetes.io/not-ready", "effect": "NoExecute", "operator": "Exists"}},
	{Key: "tolerations", ItemMatch: map[string]string{"key": "node.kubernetes.io/unreachable", "effect": "NoExecute", "operator": "Exists"}},
}
