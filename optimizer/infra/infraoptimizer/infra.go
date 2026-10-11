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
	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyInfra {
			continue
		}
		lines := compress.Lines(compress.StripANSI(b.Text))
		var out []string
		switch detect.Dialect(b.Type) {
		case "terraform":
			out = o.terraform(lines)
		case "pulumi":
			out = pulumi(lines)
		case "ansible":
			out = ansible(lines)
		case "cdk":
			out = cdk(lines)
		case "helm-diff":
			out = o.helmDiff(lines)
		default:
			out = compress.DedupeRuns(lines)
		}
		p.Blocks[i].Text = compress.Join(compress.SqueezeBlank(out))
	}
	return nil
}

var (
	// Refresh, progress, init and boilerplate lines of terraform and tofu.
	tfNoise = lazyre.New(`^(\S+: (Refreshing state\.\.\.|Reading\.\.\.|Read complete after|Still \w+\.\.\.|Creating\.\.\.|Modifying\.\.\.|Destroying\.\.\.|Creation complete after|Modifications complete after|Destruction complete after)` +
		`|(Acquiring|Releasing) state lock|Initializing (the backend|provider plugins|modules)|- (Reusing|Finding|Installing|Installed|Using previously-installed) ` +
		`|(Terraform|OpenTofu) used the selected providers|(plan\. )?Resource actions are indicated with|guarantee to take exactly these actions|─{10,}$|(Terraform|OpenTofu) will perform the following actions` +
		`|Note: You didn't use the -out option|\s+\+ create$|\s+~ update in-place$|\s+- destroy$|-/\+ destroy and then create replacement$|\+/- create replacement and then destroy$|\s+<= read \(data resources\)$)`)
	tfDone     = lazyre.New(`^\S+: (Creation|Modifications|Destruction) complete after`)
	tfHidden   = lazyre.New(`^\s*# \(\d+ unchanged (attributes|blocks|elements) hidden\)$`)
	tfKnown    = lazyre.New(`^(\s*)[+~] "?([\w.-]+)"?\s+= \(known after apply\)$`)
	tfHeader   = lazyre.New(`^  # (\S+) (will|must) be (\w+)`)
	tfBoxEdge  = lazyre.New(`^[╷╵]\s*$`)
	tfOutroEnd = lazyre.New(`^(Plan|Changes to Outputs|Apply complete!|Destroy complete!|No changes\.|Error:|Warning:)`)
)

// terraform folds noise, strips diagnostic boxes and compacts the plan body.
func (o *Optimizer) terraform(lines []string) []string {
	var out []string
	used := 0
	var block []string
	var prev string // normalized body of the last resource
	flush := func() {
		if block == nil {
			return
		}
		body := normalizedBody(block)
		if body == prev {
			out = append(out, block[0]+" (like above)")
		} else {
			out = append(out, o.resource(block, &used)...)
		}
		prev, block = body, nil
	}
	progress := 0
	for _, l := range lines {
		switch {
		case tfBoxEdge.MatchString(l):
			continue
		case strings.HasPrefix(l, "│"):
			l = strings.TrimPrefix(strings.TrimPrefix(l, "│"), " ")
		}
		if tfNoise.MatchString(l) {
			if tfDone.MatchString(l) {
				progress++
			}
			continue
		}
		switch {
		case tfHeader.MatchString(l):
			flush()
			block = []string{l}
		case block != nil && tfOutroEnd.MatchString(l):
			flush()
			out = append(out, l)
		case block != nil:
			block = append(block, l)
		default:
			out = append(out, l)
		}
	}
	flush()
	if progress > 0 {
		out = append(out, fmt.Sprintf("(%d resources completed)", progress))
	}
	return out
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
