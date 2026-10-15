package yamlx

import "testing"

func TestPrune(t *testing.T) {
	in := `{"metadata": {"name": "web", "uid": "x-1", "managedFields": [{"a": 1}]},
  "spec": {"dnsPolicy": "ClusterFirst", "image": "ghcr.io/x/web:1.2", "empty": {}, "nothing": null,
    "volumes": [{"name": "kube-api-access-abc"}, {"name": "data"}],
    "tolerations": [{"key": "a", "effect": "NoExecute"}, {"key": "b"}]}}`
	docs, err := Docs(in)
	if err != nil {
		t.Fatal(err)
	}
	rules := []Rule{
		{Key: "managedFields"}, {Key: "uid", Parent: "metadata"}, {Key: "dnsPolicy", Equals: "ClusterFirst"},
		{Key: "volumes", ItemField: "name", ItemPrefix: "kube-api-access-"},
		{Key: "tolerations", ItemMatch: map[string]string{"key": "a", "effect": "NoExecute"}},
	}
	for _, d := range docs {
		Prune(d, rules)
	}
	got, err := Encode(docs)
	if err != nil {
		t.Fatal(err)
	}
	want := `metadata:
  name: web
spec:
  image: ghcr.io/x/web:1.2
  volumes:
    - name: data
  tolerations:
    - key: b
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlain(t *testing.T) {
	for s, want := range map[string]bool{"ghcr.io/x:1": true, "a: b": false, "true": false, "123": false, "25%": true, "#x": false, "": false} {
		if got := plain(s); got != want {
			t.Errorf("plain(%q) = %v, want %v", s, got, want)
		}
	}
}
