package detect

// Families group dialects by what the output is for; each has one engine.
const (
	FamilyTest  = "test"  // test runners
	FamilyBuild = "build" // compilers, linters, type checkers, bundlers
	FamilyPkg   = "pkg"   // package managers
	FamilyTask  = "task"  // task runners, monorepo tools, CI logs
	FamilyTable = "table" // columnar CLI output
	FamilyLog   = "log"   // logs
	FamilyDoc   = "doc"   // prose and docs
	FamilyDB    = "db"    // query results, plans, migrations
	FamilyK8s   = "k8s"   // Kubernetes
	FamilyInfra = "infra" // IaC plans/applies
	FamilyCloud = "cloud" // AWS, GCP, Azure CLIs
)
