package detect

// Infrastructure as code.
const (
	Terraform = "infra/terraform" // terraform, tofu, terragrunt plan/apply/destroy
	Pulumi    = "infra/pulumi"
	Ansible   = "infra/ansible"
	CDK       = "infra/cdk"       // cdk diff/deploy/destroy
	HelmDiff  = "infra/helm-diff" // helm diff plugin
)

func init() {
	register(
		Spec{Type: Terraform, Priority: 30,
			Cmds: []string{"terraform plan", "terraform apply", "terraform destroy", "terraform refresh",
				"tofu plan", "tofu apply", "tofu destroy", "tofu refresh",
				"terragrunt plan", "terragrunt apply", "terragrunt destroy", "terragrunt run-all"},
			Sniff: anyLine(`^(\S+: (Refreshing state\.\.\.|Reading\.\.\.)|(Terraform|OpenTofu) (used the selected providers|will perform the following actions)|No changes\. Your infrastructure matches|Plan: \d+ to add)`)},
		Spec{Type: Pulumi, Cmds: []string{"pulumi up", "pulumi preview", "pulumi destroy", "pulumi refresh", "pulumi update"}, Priority: 30,
			Sniff: anyLine(`^(Previewing|Updating|Destroying|Refreshing) (update )?\(\S+\)(:|$)`)},
		Spec{Type: Ansible, Cmds: []string{"ansible-playbook", "ansible"}, Priority: 30,
			Sniff: all(anyLine(`^PLAY \[.*\] \*+$`), anyLine(`^(TASK|PLAY RECAP) .*\*+$`))},
		Spec{Type: CDK, Cmds: []string{"cdk diff", "cdk deploy", "cdk destroy"}, Priority: 30,
			Sniff: all(anyLine(`^Stack \S+$`), anyLine(`^(\[[-+~]\] |There were no differences|Resources$|IAM Statement Changes)`))},
		Spec{Type: HelmDiff, Cmds: []string{"helm diff"}, Priority: 18, // before manifests: diffs carry manifest lines
			Sniff: anyLine(`^\S+, \S+, \S+ \(\S*\) has (changed|been added|been removed):$`)},
	)
}
