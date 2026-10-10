package infraoptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress"
)

func TestTerraformFoldsPlan(t *testing.T) {
	in := `aws_s3_bucket.a: Refreshing state... [id=a]
╷
│ Error: boom
│   on main.tf line 3
╵
  # aws_sqs_queue.q[0] will be created
  + resource "aws_sqs_queue" "q" {
      + arn  = (known after apply)
      + id   = (known after apply)
      + name = "q-0"
    }
  # aws_sqs_queue.q[1] will be created
  + resource "aws_sqs_queue" "q" {
      + arn  = (known after apply)
      + id   = (known after apply)
      + name = "q-1"
    }
  # aws_instance.web will be destroyed
  - resource "aws_instance" "web" {
      - id = "i-1"
        # (12 unchanged attributes hidden)
    }
Plan: 2 to add, 0 to change, 1 to destroy.
`
	o := &Optimizer{cfg: &Config{MaxPlanLines: 300}}
	got := compress.Join(o.terraform(compress.Lines(in)))
	for _, want := range []string{"Error: boom\n  on main.tf line 3\n", "(known after apply: arn, id)", `"q-0"`,
		`aws_sqs_queue.q[1] will be created (like above)`, `- id = "i-1"`, "Plan: 2 to add"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, noise := range []string{"Refreshing", "│", "╷", "unchanged attributes", `"q-1"`} {
		if strings.Contains(got, noise) {
			t.Errorf("kept %q in:\n%s", noise, got)
		}
	}
}

func TestAnsibleKeepsFailuresAndRecap(t *testing.T) {
	in := `PLAY [web] *****
TASK [ping] *****
ok: [a]
ok: [b]
TASK [copy] *****
changed: [b]
ok: [a]
TASK [check] *****
fatal: [b]: FAILED! => {"msg": "nope"}
ok: [a]
PLAY RECAP *****
a                          : ok=3    changed=0    unreachable=0    failed=0
b                          : ok=2    changed=1    unreachable=0    failed=1
`
	got := compress.Join(ansible(compress.Lines(in)))
	want := `PLAY [web]
TASK [copy] changed: b (ok/skipped ×1)
TASK [check]
fatal: [b]: FAILED! => {"msg": "nope"}
PLAY RECAP
a                          : ok=3    changed=0    unreachable=0    failed=0
b                          : ok=2    changed=1    unreachable=0    failed=1
… 1 ok/skipped-only task omitted
`
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestPulumiDropsProgress(t *testing.T) {
	in := "@ updating....\n +  aws:sqs:Queue q creating (0s) \n +  aws:sqs:Queue q created (1s) \n    aws:iam:Role r  \nResources:\n"
	got := compress.Join(pulumi(compress.Lines(in)))
	if got != " +  aws:sqs:Queue q created (1s) \nResources:\n… 1 unchanged resource row omitted\n" {
		t.Fatalf("got %q", got)
	}
}
