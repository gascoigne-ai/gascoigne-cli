package infraoptimizer

import (
	"context"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// TestDispatch pins which handler each dialect reaches (and the fallback).
func TestDispatch(t *testing.T) {
	text := `aws_s3_bucket.a: Refreshing state... [id=a]
Initializing the backend...
- Installing hashicorp/aws v5.31.0
  # aws_sqs_queue.q[0] will be created
  + resource "aws_sqs_queue" "q" {
      + arn  = (known after apply)
      + name = "q-0"
    }
  # aws_sqs_queue.q[1] will be created
  + resource "aws_sqs_queue" "q" {
      + arn  = (known after apply)
      + name = "q-1"
    }
Plan: 2 to add, 0 to change, 0 to destroy.
module.a.aws_instance.x
module.a.aws_instance.y
Warning: Deprecated
Warning: Deprecated
TASK [web : install] ******
ok: [h1]
ok: [h2]
fatal: [h3]: FAILED! => {"msg": "boom"}
PLAY RECAP ****
h1 : ok=1 changed=0 unreachable=0 failed=0
Updating (dev):
 +  aws:s3:Bucket b create
 @ updating....
Resources:
    + 1 to create
same
same
same
`
	o := &Optimizer{cfg: &Config{MaxPlanLines: 300, DiffContext: 3}}
	want := map[string]func([]string) []string{
		"terraform":          o.terraform,
		"terraform-init":     terraformInit,
		"terraform-validate": tfBoxes,
		"terraform-state":    terraformState,
		"terraform-output":   terraformOutput,
		"pulumi":             pulumi,
		"ansible":            ansible,
		"cdk":                cdk,
		"helm-diff":          o.helmDiff,
		"unknown":            compress.DedupeRuns,
	}
	for dialect, handler := range want {
		t.Run(dialect, func(t *testing.T) {
			p := payload.New(payload.CmdOutput, text)
			p.Blocks[0].Type = "infra/" + dialect
			if err := o.Optimize(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			exp := compress.Join(compress.SqueezeBlank(handler(compress.CleanLines(text))))
			if p.Blocks[0].Text != exp {
				t.Errorf("got:\n%s\nwant:\n%s", p.Blocks[0].Text, exp)
			}
		})
	}
}
