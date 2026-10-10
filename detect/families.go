package detect

// Families group dialects by what the output is for; each has one engine.
const (
	FamilyTest   = "test"   // test runners
	FamilyBuild  = "build"  // compilers, linters, type checkers, bundlers
	FamilyPkg    = "pkg"    // package managers
	FamilyTask   = "task"   // task runners, monorepo tools, CI logs
	FamilyVCS    = "vcs"    // git, forges
	FamilySearch = "search" // grep and friends
	FamilyFS     = "fs"     // listings, disk usage, source files
	FamilyTable  = "table"  // columnar CLI output
	FamilyData   = "data"   // structured data
	FamilyLog    = "log"    // logs
	FamilyDoc    = "doc"    // prose and docs
	FamilyWeb    = "web"    // web pages, HTTP
	FamilyDB     = "db"     // query results, plans, migrations
	FamilyK8s    = "k8s"    // Kubernetes
	FamilyInfra  = "infra"  // IaC plans/applies
	FamilyCloud  = "cloud"  // AWS, GCP, Azure CLIs
)

// Families lists every family.
var Families = []string{
	FamilyTest, FamilyBuild, FamilyPkg, FamilyTask, FamilyVCS, FamilySearch, FamilyFS, FamilyTable,
	FamilyData, FamilyLog, FamilyDoc, FamilyWeb, FamilyDB, FamilyK8s, FamilyInfra, FamilyCloud,
}
