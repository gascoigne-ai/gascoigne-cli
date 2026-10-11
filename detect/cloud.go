package detect

import "github.com/gascoigne-ai/gascoigne-cli/lazyre"

// Cloud CLIs.
const (
	AWS   = "cloud/aws"
	AWSS3 = "cloud/aws-s3" // aws s3 ls object listings
	GCP   = "cloud/gcp"
	Azure = "cloud/azure"
)

func init() {
	register(
		Spec{Type: AWS, Cmds: []string{"aws", "sam", "cdk list"}, Priority: 18, Refines: []string{JSON},
			Sniff: func(t string, l []string) bool {
				return isJSON(t, l) && containsAny(t[:min(len(t), 8192)], `"arn:aws`, `"ResponseMetadata"`, `"Reservations"`, `"StackEvents"`, `"OwnerId"`)
			}},
		Spec{Type: AWSS3, Cmds: []string{"aws s3 ls", "s5cmd ls"}, Priority: 60, Sniff: ratio(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\s+\d+ \S|\s+PRE \S+/$)`, 0.8)},
		Spec{Type: GCP, Cmds: []string{"gcloud", "gsutil", "bq"}, Priority: 18, Refines: []string{JSON, YAML},
			Sniff: func(t string, l []string) bool {
				head := t[:min(len(t), 8192)]
				return containsAny(head, "https://www.googleapis.com/compute/", `"kind": "compute#`, "kind: compute#", "serving.knative.dev/", "run.googleapis.com/")
			}},
		Spec{Type: Azure, Cmds: []string{"az", "azd"}, Priority: 18, Refines: []string{JSON},
			Sniff: func(t string, l []string) bool { return isJSON(t, l) && azureID.MatchString(t[:min(len(t), 8192)]) }},
	)
}

var azureID = lazyre.New(`"/subscriptions/[0-9a-f-]{36}/resourceGroups/`)
