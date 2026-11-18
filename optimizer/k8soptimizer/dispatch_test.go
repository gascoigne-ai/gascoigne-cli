package k8soptimizer

import (
	"context"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// TestDispatch pins which handler each dialect reaches (and the fallback).
func TestDispatch(t *testing.T) {
	text := `NAME    READY   STATUS             RESTARTS   AGE
web-1   1/1     Running            0          5d
web-2   0/1     CrashLoopBackOff   9          5d
web-3   1/1     Running            0          5d
Name:         web
Namespace:    default
Events:
  Type    Reason   Age   From     Message
  ----    ------   ----  ----     -------
  Normal  Pulled   5m    kubelet  Pulled image
deployment.apps/a unchanged
deployment.apps/b unchanged
deployment.apps/c configured
Waiting for deployment "web" rollout to finish: 1 of 3 updated replicas are available...
Waiting for deployment "web" rollout to finish: 2 of 3 updated replicas are available...
deployment "web" successfully rolled out
NAME: rel
STATUS: deployed
NOTES:
1. hello
`
	o := &Optimizer{cfg: &Config{MaxItems: 6, Limits: table.Limits{MaxRows: 30, MaxCell: 60}, NotesLines: 5}}
	topts := o.cfg.Options(nil)
	want := map[string]func([]string) []string{
		"describe": describe,
		"events":   func(lines []string) []string { return events(lines, topts) },
		"apply":    apply,
		"rollout":  rollout,
		"helm":     func(lines []string) []string { return o.helm(lines, topts) },
		"get": func(lines []string) []string {
			if out, ok := table.Compress(lines, topts); ok {
				return out
			}
			return lines
		},
		"unknown": func(lines []string) []string {
			if out, ok := table.Compress(lines, topts); ok {
				return out
			}
			return lines
		},
	}
	for dialect, handler := range want {
		t.Run(dialect, func(t *testing.T) {
			p := payload.New(payload.CmdOutput, text)
			p.Blocks[0].Type = "k8s/" + dialect
			if err := o.Optimize(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			exp := compress.Join(handler(compress.CleanLines(text)))
			if p.Blocks[0].Text != exp {
				t.Errorf("got:\n%s\nwant:\n%s", p.Blocks[0].Text, exp)
			}
		})
	}
	t.Run("manifest", func(t *testing.T) {
		doc := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: a\n  managedFields:\n  - manager: x\n"
		p := payload.New(payload.CmdOutput, doc)
		p.Blocks[0].Type = "k8s/manifest"
		if err := o.Optimize(context.Background(), p); err != nil {
			t.Fatal(err)
		}
		exp, err := o.manifests(doc)
		if err != nil || p.Blocks[0].Text != exp || exp == doc {
			t.Errorf("got %q want %q err %v", p.Blocks[0].Text, exp, err)
		}
	})
}
