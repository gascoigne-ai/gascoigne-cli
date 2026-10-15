package detect

import "slices"

// Kubernetes.
const (
	K8sGet      = "k8s/get"      // kubectl get/top tables
	K8sManifest = "k8s/manifest" // -o yaml/json, helm template, kustomize
	K8sDescribe = "k8s/describe"
	K8sEvents   = "k8s/events"
	Helm        = "k8s/helm" // list, status, history, install/upgrade
)

func init() {
	register(
		Spec{Type: K8sGet, Cmds: []string{"kubectl get", "kubectl top", "oc get", "k get"}, Priority: 46,
			Sniff: firstLine(`^(NAMESPACE\s+)?NAME\s+(READY\s+STATUS|STATUS\s+ROLES|TYPE\s+CLUSTER-IP|READY\s+UP-TO-DATE|DATA\s+AGE|COMPLETIONS)`)},
		Spec{Type: K8sManifest, Cmds: []string{"helm template", "kustomize build", "kubectl kustomize"}, Priority: 19, Refines: []string{YAML, JSON},
			CmdFunc: func(args []string) bool {
				return slices.Contains([]string{"kubectl", "oc", "k"}, args[0]) && slices.Contains([]string{"yaml", "json"}, flagValue(args, "-o", "--output"))
			},
			Sniff: func(t string, l []string) bool {
				return yamlManifest(t, l) || isJSON(t, l) && containsAll(t[:min(len(t), 4096)], `"apiVersion"`, `"kind"`)
			}},
		Spec{Type: K8sDescribe, Cmds: []string{"kubectl describe", "oc describe", "k describe"}, Priority: 46,
			Sniff: all(anyLine(`^Name:\s+\S`), anyLine(`^Namespace:\s+\S`), anyLine(`^Events:`))},
		Spec{Type: K8sEvents, Cmds: []string{"kubectl get events", "kubectl events", "oc get events"}, Priority: 45,
			Sniff: firstLine(`^(NAMESPACE\s+)?LAST SEEN\s+TYPE\s+REASON\s+OBJECT`)},
		Spec{Type: Helm, Cmds: []string{"helm list", "helm ls", "helm status", "helm history", "helm install", "helm upgrade", "helm rollback"}, Priority: 46,
			Sniff: all(anyLine(`^LAST DEPLOYED: `), anyLine(`^STATUS: `))},
	)
}

var yamlManifest = all(anyLine(`^\s*(- )?apiVersion: [\w./]+$`), anyLine(`^\s*kind: [A-Z]\w+$`))
