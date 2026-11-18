// Package infraoptimizer is the infra family engine for infrastructure as
// code: plans and applies (terraform, tofu, pulumi, cdk, helm diff) keep the
// changes, errors and summary while refresh, progress and "known after apply"
// noise is folded; ansible runs keep changed and failed tasks plus the recap.
package infraoptimizer

import (
	"context"
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

// handlers compress one dialect; others fall back to DedupeRuns.
var handlers = map[string]func(opt *Optimizer, lines []string) []string{
	"terraform":          (*Optimizer).terraform,
	"terraform-init":     func(_ *Optimizer, lines []string) []string { return terraformInit(lines) },
	"terraform-validate": func(_ *Optimizer, lines []string) []string { return tfBoxes(lines) },
	"terraform-state":    func(_ *Optimizer, lines []string) []string { return terraformState(lines) },
	"terraform-output":   func(_ *Optimizer, lines []string) []string { return terraformOutput(lines) },
	"pulumi":             func(_ *Optimizer, lines []string) []string { return pulumi(lines) },
	"ansible":            func(_ *Optimizer, lines []string) []string { return ansible(lines) },
	"cdk":                func(_ *Optimizer, lines []string) []string { return cdk(lines) },
	"helm-diff":          (*Optimizer).helmDiff,
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	return data.RewriteFamily(func(block payload.Block) (string, error) {
		lines := compress.CleanLines(block.Text)
		handle, ok := handlers[detect.Dialect(block.Type)]
		if !ok {
			handle = func(_ *Optimizer, lines []string) []string { return compress.DedupeRuns(lines) }
		}
		out := handle(o, lines)
		return compress.Join(compress.SqueezeBlank(out)), nil
	}, detect.FamilyInfra)
}

var (
	// Refresh, progress, init and boilerplate lines of terraform and tofu.
	tfNoise = pcomb.Rules{
		{
			Name: "resource progress",
			Match: afterLabel(pcomb.Any(pcomb.Prefix("Refreshing state...", "Reading...", "Read complete after", "Creating...", "Modifying...", "Destroying...",
				"Creation complete after", "Modifications complete after", "Destruction complete after"), pcomb.Re(`^Still \w+\.\.\.`))),
			Yes: []string{"aws_instance.web: Refreshing state... [id=i-0abc]", "module.db.aws_db_instance.main: Still creating... [10s elapsed]", "aws_s3_bucket.b: Creation complete after 2s [id=b]", "data.aws_ami.u: Read complete after 0s [id=ami-1]"},
			No:  []string{"Refreshing state...", "aws_instance.web : Refreshing state...", "aws_instance.web:  Refreshing state...", "aws_instance.web: Still ...", "aws_instance.web: Creating"},
		},
		{
			Name:  "state lock",
			Match: pcomb.Prefix("Acquiring state lock", "Releasing state lock"),
			Yes:   []string{"Acquiring state lock. This may take a few moments...", "Releasing state lock. This may take a few moments..."},
			No:    []string{"Acquiring the lock"},
		},
		{
			Name:  "init",
			Match: pcomb.Prefix("Initializing the backend", "Initializing provider plugins", "Initializing modules"),
			Yes:   []string{"Initializing the backend...", "Initializing provider plugins...", "Initializing modules..."},
			No:    []string{"Initializing"},
		},
		{
			Name:  "provider install",
			Match: pcomb.Prefix("- Reusing ", "- Finding ", "- Installing ", "- Installed ", "- Using previously-installed "),
			Yes:   []string{"- Reusing previous version of hashicorp/aws from the dependency lock file", "- Installed hashicorp/aws v5.31.0 (signed by HashiCorp)"},
			No:    []string{"- Reusing", "-Installing x"},
		},
		{
			Name: "plan boilerplate",
			Match: pcomb.Prefix("Terraform used the selected providers", "OpenTofu used the selected providers", "Resource actions are indicated with",
				"plan. Resource actions are indicated with", "guarantee to take exactly these actions", "Terraform will perform the following actions",
				"OpenTofu will perform the following actions", "Note: You didn't use the -out option"),
			Yes: []string{"Terraform used the selected providers to generate the following execution plan. Resource actions are indicated with the following symbols:",
				"guarantee to take exactly these actions if you run \"terraform apply\" now.", "Terraform will perform the following actions:", "Note: You didn't use the -out option to save this plan, so Terraform can't guarantee to take exactly these actions"},
			No: []string{"Terraform will apply", " Terraform will perform the following actions:"},
		},
		{
			Name:  "ruler",
			Match: pcomb.Re(`^─{10,}$`),
			Yes:   []string{"─────────────────────────────────────────────────────────────────────────────", "──────────"},
			No:    []string{"─────────", "──────────x"},
		},
		{
			Name: "action legend",
			Match: pcomb.Any(
				pcomb.Indented(pcomb.Exact("+ create", "~ update in-place", "- destroy", "<= read (data resources)")),
				pcomb.Exact("-/+ destroy and then create replacement", "+/- create replacement and then destroy"),
			),
			Yes: []string{"  + create", "  ~ update in-place", "  - destroy", "  <= read (data resources)", "-/+ destroy and then create replacement", "+/- create replacement and then destroy"},
			No:  []string{"+ create", "  + create x", "  -/+ destroy and then create replacement"},
		},
	}
	tfDone     = lazyre.New(`^\S+: (Creation|Modifications|Destruction) complete after`)
	tfHidden   = lazyre.New(`^\s*# \(\d+ unchanged (attributes|blocks|elements) hidden\)$`)
	tfKnown    = lazyre.New(`^(\s*)[+~] "?([\w.-]+)"?\s+= \(known after apply\)$`)
	tfHeader   = lazyre.New(`^  # (\S+) (will|must) be (\w+)`)
	tfBoxEdge  = lazyre.New(`^[╷╵]\s*$`)
	tfOutroEnd = lazyre.New(`^(Plan|Changes to Outputs|Apply complete!|Destroy complete!|No changes\.|Error:|Warning:)`)
)

// terraform folds noise, strips diagnostic boxes and compacts the plan body.
func (o *Optimizer) terraform(lines []string) []string {
	run := &tfRun{o: o}
	for _, l := range lines {
		run.line(l)
	}
	run.flush()
	if run.progress > 0 {
		run.out = append(run.out, fmt.Sprintf("(%d resources completed)", run.progress))
	}
	return run.out
}

// tfRun is the state of terraform: the plan entry being read and the
// output so far.
type tfRun struct {
	o        *Optimizer
	out      []string
	used     int
	block    []string
	prev     string // normalized body of the last resource
	progress int    // finished-resource lines folded away
}

// flush writes the plan entry being read: compacted, or as "like above"
// when its body equals the previous one.
func (run *tfRun) flush() {
	if run.block == nil {
		return
	}
	body := normalizedBody(run.block)
	if body == run.prev {
		run.out = append(run.out, run.block[0]+" (like above)")
	} else {
		run.out = append(run.out, run.o.resource(run.block, &run.used)...)
	}
	run.prev, run.block = body, nil
}

// line reads one line: box edges and noise go, headers start an entry.
func (run *tfRun) line(line string) {
	if tfBoxEdge.MatchString(line) {
		return
	}
	if strings.HasPrefix(line, "│") {
		line = strings.TrimPrefix(strings.TrimPrefix(line, "│"), " ")
	}
	if tfNoise.Match(line) {
		if tfDone.MatchString(line) {
			run.progress++
		}
		return
	}
	switch {
	case tfHeader.MatchString(line):
		run.flush()
		run.block = []string{line}
	case run.block != nil && tfOutroEnd.MatchString(line):
		run.flush()
		run.out = append(run.out, line)
	case run.block != nil:
		run.block = append(run.block, line)
	default:
		run.out = append(run.out, line)
	}
}

// normalizedBody is a resource body with values reduced to their shape, so
// resources made by count or for_each compare equal.
func normalizedBody(block []string) string {
	var b strings.Builder
	for _, l := range block[1:] {
		b.WriteString(compress.Normalize(l) + "\n")
	}
	return b.String()
}

// resource compacts one plan entry: hidden-attribute notes go, attributes
// known after apply become one line, and past the budget only the header of
// creates and in-place updates stays.
func (o *Optimizer) resource(block []string, used *int) []string {
	action := tfHeader.FindStringSubmatch(block[0])[3]
	var out, known []string
	at, indent := -1, ""
	for _, l := range block {
		if tfHidden.MatchString(l) {
			continue
		}
		if m := tfKnown.FindStringSubmatch(l); m != nil {
			if at < 0 {
				at, indent = len(out), m[1]
				out = append(out, "")
			}
			known = append(known, m[2])
			continue
		}
		out = append(out, l)
	}
	if at >= 0 {
		out[at] = indent + "  (known after apply: " + strings.Join(known, ", ") + ")"
	}
	keep := action == "destroyed" || action == "replaced" || *used+len(out) <= o.cfg.MaxPlanLines
	if !keep {
		return []string{block[0] + " " + compress.Omitted(len(out)-1, "lines")}
	}
	*used += len(out)
	return out
}

// afterLabel matches `<label>: ` (a first word ending in a colon) followed by
// text that matcher matches: the regexp `^\S+: m`.
func afterLabel(matcher pcomb.Matcher) pcomb.Matcher {
	return func(line string) bool {
		word, rest, ok := strings.Cut(line, " ")
		return ok && len(word) > 1 && strings.HasSuffix(word, ":") && !strings.ContainsAny(word, "\t\n\f\r") && matcher(rest)
	}
}
