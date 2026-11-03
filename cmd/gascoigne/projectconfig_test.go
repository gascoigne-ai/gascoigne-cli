package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
)

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProjectConfigPath(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	deep := filepath.Join(repo, "a", "b")
	plain := filepath.Join(root, "plain", "x")
	mkdirs(t, deep, plain, filepath.Join(root, "gitroot", "pkg"))
	write(t, filepath.Join(repo, ".git"), "gitdir: elsewhere") // a worktree's .git is a file
	write(t, filepath.Join(root, projectConfigName), "")       // above the repo: never applies
	write(t, filepath.Join(repo, "a", projectConfigName), "")
	write(t, filepath.Join(root, "gitroot", ".git", "HEAD"), "")
	write(t, filepath.Join(root, "gitroot", projectConfigName), "")

	tests := []struct{ name, dir, want string }{
		{"nearest wins", deep, filepath.Join(repo, "a", projectConfigName)},
		{"stops at git root", repo, ""},
		{"file at the git root", filepath.Join(root, "gitroot", "pkg"), filepath.Join(root, "gitroot", projectConfigName)},
		{"no git root: cwd only", plain, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectConfigPath(tt.dir); got != tt.want {
				t.Errorf("projectConfigPath(%s) = %q, want %q", tt.dir, got, tt.want)
			}
		})
	}
}

func TestWithProjectOverlay(t *testing.T) {
	sandbox(t)
	repo := t.TempDir()
	mkdirs(t, filepath.Join(repo, ".git"))
	t.Chdir(repo)
	write(t, filepath.Join(repo, projectConfigName), "router:\n  default: [git]\n")

	base, err := configFile("")
	if err != nil {
		t.Fatal(err)
	}
	data, err := withProject(base)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"git"}; !slices.Equal(cfg.Router.Default, want) {
		t.Errorf("router.default = %v, want %v from the project file", cfg.Router.Default, want)
	}
	if len(cfg.Pipelines) == 0 {
		t.Error("project file dropped the pipelines it did not set")
	}
	// The server keys its warm pipeline on this text.
	if string(data) == string(base) {
		t.Error("project file left the config text unchanged: the server hash would not tell repos apart")
	}
}

// A project file with a misspelled key must fail naming the file, not pass
// as the merged document's unknown field.
func TestWithProjectBadKey(t *testing.T) {
	sandbox(t)
	repo := t.TempDir()
	t.Chdir(repo)
	path := filepath.Join(repo, projectConfigName)
	write(t, path, "router:\n  defaults: [git]\n")

	base, err := configFile("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = withProject(base)
	if err == nil || !strings.Contains(err.Error(), projectConfigName) || !strings.Contains(err.Error(), "defaults") {
		t.Errorf("err = %v, want it to name %s and the bad key", err, projectConfigName)
	}
}

func TestWithProjectNone(t *testing.T) {
	sandbox(t)
	base := []byte("a: 1\n")
	if got, err := withProject(base); err != nil || string(got) != string(base) {
		t.Errorf("got %q, %v; want data unchanged", got, err)
	}
}

// A repo's .gascoigne.yaml is not trusted code: an external optimizer there
// would run the repo's command on whoever opens it with an agent.
func TestWithProjectRejectsExternalOptimizer(t *testing.T) {
	sandbox(t)
	repo := t.TempDir()
	t.Chdir(repo)
	write(t, filepath.Join(repo, projectConfigName), "optimizers:\n  external/squash:\n    command: [./squash.sh]\n")

	base, err := configFile("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = withProject(base)
	if err == nil || !strings.Contains(err.Error(), "external/squash") || !strings.Contains(err.Error(), projectConfigName) {
		t.Errorf("err = %v, want it to refuse external/squash and name %s", err, projectConfigName)
	}
}
