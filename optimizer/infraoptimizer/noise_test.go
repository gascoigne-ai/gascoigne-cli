package infraoptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

func TestNoiseRulesVerify(t *testing.T) {
	for name, rules := range map[string]interface{ Verify() error }{"tfNoise": tfNoise, "cdkNoise": cdkNoise} {
		if err := rules.Verify(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

// The regexps tfNoise and cdkNoise replaced, kept to pin the old behavior.
var (
	oldTfNoise = lazyre.New(`^(\S+: (Refreshing state\.\.\.|Reading\.\.\.|Read complete after|Still \w+\.\.\.|Creating\.\.\.|Modifying\.\.\.|Destroying\.\.\.|Creation complete after|Modifications complete after|Destruction complete after)` +
		`|(Acquiring|Releasing) state lock|Initializing (the backend|provider plugins|modules)|- (Reusing|Finding|Installing|Installed|Using previously-installed) ` +
		`|(Terraform|OpenTofu) used the selected providers|(plan\. )?Resource actions are indicated with|guarantee to take exactly these actions|─{10,}$|(Terraform|OpenTofu) will perform the following actions` +
		`|Note: You didn't use the -out option|\s+\+ create$|\s+~ update in-place$|\s+- destroy$|-/\+ destroy and then create replacement$|\+/- create replacement and then destroy$|\s+<= read \(data resources\)$)`)
	oldCdkNoise = lazyre.New(`^(✨\s+(Synthesis time|Total time|Deployment time|Number of stacks with differences)|\S+: (deploying|creating CloudFormation changeset|building|publishing|assets? (built|published))|\[\d+%\] (start|success|check|upload|build|cached))`)
)

func TestTfNoiseSameAsRegexp(t *testing.T) {
	var ex []string
	for _, r := range tfNoise {
		ex = append(ex, r.Yes...)
		ex = append(ex, r.No...)
	}
	ex = append(ex, "aws_instance.web: Refreshing state... [id=i-1]", "a:b: Refreshing state...", "a: Refreshing state...", ": Refreshing state...", "a:  Refreshing state...", "a:\tRefreshing state...", "a b: Refreshing state...",
		"module.x.aws_s3.b: Still creating... [10s elapsed]", "x: Still 1_a... [x]", "x: Still ...", "x: Still a...", "x: Reading...", "x: Read complete after 1s", "x: Creating...", "x: Modifying...", "x: Destroying...",
		"x: Creation complete after 2s", "x: Modifications complete after 2s", "x: Destruction complete after 2s", "x: Reading", "Acquiring state lock. This may take a few moments...", "Releasing state lock. This may take a few moments...",
		"Initializing the backend...", "Initializing provider plugins...", "Initializing modules...", "Initializing", "- Reusing previous version of hashicorp/aws from the dependency lock file", "- Finding hashicorp/aws versions matching", "- Installing x", "- Installed x", "- Using previously-installed x", "- Reusing",
		"Terraform used the selected providers to generate the following execution plan.", "OpenTofu used the selected providers", "Resource actions are indicated with the following symbols:", "plan. Resource actions are indicated with", "xplan. Resource actions are indicated with",
		"guarantee to take exactly these actions if you run \"terraform apply\" now.", "─────────────────────────────────────────────────────────────────────────────", "─────────", "──────────", "───────────x", "x──────────",
		"Terraform will perform the following actions:", "OpenTofu will perform the following actions:", "Note: You didn't use the -out option to save this plan", "  + create", "+ create", "  + create ", "\t+ create", "  ~ update in-place", "  - destroy", "-/+ destroy and then create replacement", "+/- create replacement and then destroy", "  <= read (data resources)", "<= read (data resources)", "  -/+ destroy and then create replacement", "  - destroy x")
	sameAsOld(t, oldTfNoise.MatchString, tfNoise.Match, ex)
}

func TestCdkNoiseSameAsRegexp(t *testing.T) {
	var ex []string
	for _, r := range cdkNoise {
		ex = append(ex, r.Yes...)
		ex = append(ex, r.No...)
	}
	ex = append(ex, "✨  Synthesis time: 3.2s", "✨ Total time: 5s", "✨Total time: 5s", "✨\tDeployment time: 1s", "✨  Number of stacks with differences: 1", "✨  Done", "✨",
		"MyStack: deploying... [1/2]", "MyStack: creating CloudFormation changeset...", "x: building assets...", "x: publishing assets...", "x: asset built", "x: assets built", "x: asset published", "x: assets published", "x: assetsx built", "x: asset  built", "x:deploying", "a b: deploying",
		"[0%] start: Publishing abc", "[100%] success: Published abc", "[50%] check: x", "[50%] upload: x", "[50%] build: x", "[50%] cached: x", "[%] start", "[5%] starting", "[5%]start", "[x%] start")
	sameAsOld(t, oldCdkNoise.MatchString, cdkNoise.Match, ex)
}
