package infraoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

func TestTerraformInitKeepsModulesAndStatus(t *testing.T) {
	in := "Initializing the backend...\n\nSuccessfully configured the backend \"s3\"! Terraform will automatically\nuse this backend unless the backend configuration changes.\nInitializing modules...\nDownloading registry.terraform.io/x/vpc/aws 5.5.1 for vpc...\n- vpc in .terraform/modules/vpc\n\nInitializing provider plugins...\n- Finding hashicorp/aws versions matching \"~> 5.0\"...\n- Installing hashicorp/aws v5.31.0...\n- Installed hashicorp/aws v5.31.0 (signed by HashiCorp)\n- Installed acme/odd v1.0.0 (unauthenticated)\n\nTerraform has created a lock file .terraform.lock.hcl to record the provider\nselections it made above.\n\nTerraform has been successfully initialized!\n\nYou may now begin working with Terraform. Try running \"terraform plan\".\n\nIf you ever set or change modules or backend configuration for Terraform,\nrerun this command.\n"
	got := compress.Join(compress.SqueezeBlank(terraformInit(compress.Lines(in))))
	for _, want := range []string{"Downloading registry.terraform.io/x/vpc/aws 5.5.1 for vpc", "- vpc in .terraform/modules/vpc",
		"providers: hashicorp/aws v5.31.0, acme/odd v1.0.0 (unauthenticated)", "successfully initialized!"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, noise := range []string{"Finding", "lock file", "You may now", "rerun this"} {
		if strings.Contains(got, noise) {
			t.Errorf("kept %q in:\n%s", noise, got)
		}
	}
}

func TestTerraformStateGroupsByModule(t *testing.T) {
	in := "aws_s3_bucket.a[0]\naws_s3_bucket.a[1]\naws_s3_bucket.a[2]\nmodule.vpc.aws_vpc.this[0]\nmodule.vpc.aws_subnet.p[\"x\"]\nmodule.vpc.aws_subnet.p[\"y\"]\nNo state file was found!\n"
	got := compress.Join(terraformState(compress.Lines(in)))
	want := "No state file was found!\n(root): aws_s3_bucket.a[×3]\nmodule.vpc: aws_vpc.this[0], aws_subnet.p[\"x\",\"y\"]\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTerraformBoxesFoldRepeatedWarnings(t *testing.T) {
	box := func(file string, n int) string {
		return fmt.Sprintf("╷\n│ Warning: Argument is deprecated\n│ \n│   with r.%s,\n│   on %s line %d, in resource \"r\" \"x\":\n│    %d:   acl = \"p\"\n│ \n│ Use the other resource\n╵\n", file, file, n, n)
	}
	in := box("a.tf", 4) + box("b.tf", 9) + "╷\n│ Error: Missing\n│ \n│   on c.tf line 1:\n│ \n│ nope\n╵\n"
	got := compress.Join(compress.SqueezeBlank(tfBoxes(compress.Lines(in))))
	for _, want := range []string{"Warning: Argument is deprecated (×2, also b.tf:9)", "on a.tf line 4", "Error: Missing", "on c.tf line 1:"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "b.tf line") || strings.Contains(got, "│") {
		t.Errorf("repeat or box kept:\n%s", got)
	}
}

func TestTerraformOutputJoinsPlainLists(t *testing.T) {
	in := "ids = [\n  \"a\",\n  \"b\",\n]\nnested = [\n  {\n    \"x\" = 1\n  },\n]\nraw = \"v\"\n"
	got := compress.Join(terraformOutput(compress.Lines(in)))
	want := "ids = [\"a\", \"b\"]\nnested = [\n  {\n    \"x\" = 1\n  },\n]\nraw = \"v\"\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	json := "{\n  \"a\": {\n    \"value\": 1\n  }\n}\n"
	if got := compress.Join(terraformOutput(compress.Lines(json))); got != json {
		t.Errorf("-json output changed:\n%s", got)
	}
}
