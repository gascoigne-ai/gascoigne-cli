package detect

import "testing"

func TestFromCmd(t *testing.T) {
	tests := []struct{ cmd, want string }{
		{"go test ./...", GoTest},
		{"go test -json ./...", GoTestJSON},
		{"CGO_ENABLED=0 go build ./cmd/x", GoBuild},
		{"/usr/bin/git -C repo status", GitStatus},
		{"git log --oneline", GitLog},
		{"git show HEAD", Diff},
		{"git push origin main", GitRemote},
		{"git grep -n foo", Grep},
		{"npm install", NPM},
		{"pnpm test", ""},
		{"pnpm install", PNPM},
		{"pip install -r requirements.txt", Pip},
		{"sudo apt-get install -y jq", OSPkg},
		{"go get example.com/x", GoMod},
		{"bun test", BunTest},
		{"bun install", BunInstall},
		{"kubectl -n prod get pods", K8sGet},
		{"kubectl get pods -o yaml", K8sManifest},
		{"kubectl get deploy -oyaml", K8sManifest},
		{"kubectl get events -A", K8sEvents},
		{"kubectl describe pod web-1", K8sDescribe},
		{"helm status shop", Helm},
		{"npx --yes nx run-many -t build", NX},
		{"npx nx run-many -t build", NX},
		{"npx tsc --noEmit", TSC},
		{"npx vitest run", Vitest},
		{"uv run pytest -x", Pytest},
		{"python3 -m pytest", Pytest},
		{"bundle exec rspec", RSpec},
		{"cargo test", CargoTest},
		{"rg -n TODO", Grep},
		{"sudo ls -la /usr/lib", LS},
		{"cat main.go", Code},
		{"cat data.json", JSON},
		{"head -50 app.log", Log},
		{"go run .", ""},
		{"make", Make},
		{"", ""},
	}
	for _, tt := range tests {
		if got := FromCmd(tt.cmd); got != tt.want {
			t.Errorf("FromCmd(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestCommand(t *testing.T) {
	tests := map[string]string{
		"go test -v ./...":         "go test",
		"/usr/bin/git -C x log -3": "git log",
		"npx nx run-many -t build": "nx run-many",
		"ls -la /usr/lib":          "ls",
		"cat a/b.go":               "cat",
		"FOO=1 sudo npm ci":        "npm ci",
		"":                         "",
	}
	for in, want := range tests {
		if got := Command(in); got != want {
			t.Errorf("Command(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFromFileAndMIME(t *testing.T) {
	if got := FromFile("/a/b.TSX"); got != Code {
		t.Errorf("FromFile tsx = %q", got)
	}
	if got := FromFile("x.unknown"); got != "" {
		t.Errorf("FromFile unknown = %q", got)
	}
	if got := FromMIME("text/html; charset=utf-8"); got != HTML {
		t.Errorf("FromMIME html = %q", got)
	}
}

func TestSniff(t *testing.T) {
	tests := []struct{ name, text, want string }{
		{"empty", " \n", Text},
		{"json", `{"a": [1, 2]}`, JSON},
		{"json array", "[\n {\"a\":1}\n]\n", JSON},
		{"ndjson", "{\"a\":1}\n{\"a\":2}\n", NDJSON},
		{"json stream", "{\n \"a\": 1\n}\n{\n \"a\": 2\n}\n", JSON},
		{"log not grep", "2026-10-01T12:00:00.1Z INFO a\n2026-10-01T12:00:01.2Z INFO b\n", Log},
		{"go test json", `{"Time":"x","Action":"run","Package":"p","Test":"T"}` + "\n", GoTestJSON},
		{"html", "<!DOCTYPE html>\n<html><body>x</body></html>", HTML},
		{"diff", "diff --git a/x b/x\nindex 1..2\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n", Diff},
		{"git log", "commit 0123456789abcdef0123456789abcdef01234567\nAuthor: A <a@b>\n", GitLog},
		{"git status", "On branch main\nnothing to commit, working tree clean\n", GitStatus},
		{"go test", "=== RUN   TestX\n--- PASS: TestX (0.00s)\nPASS\nok  \tpkg\t0.1s\n", GoTest},
		{"go build", "# example.com/x\n./main.go:3:2: undefined: foo\n", GoBuild},
		{"tsc", "src/a.ts(3,5): error TS2322: Type 'string' is not assignable\n", TSC},
		{"bun", "bun test v1.2.0 (abc)\n", BunTest},
		{"nx", "\n > nx run app:build\n\n NX   Successfully ran target build for 3 projects\n", NX},
		{"jest", "PASS src/a.test.ts\nFAIL src/b.test.ts\nTests:       1 failed, 3 passed, 4 total\n", JSTest},
		{"npm", "npm warn deprecated inflight@1.0.6: leaks\nadded 120 packages in 3s\n", NPM},
		{"npm verbose", "npm http fetch GET 200 https://registry.npmjs.org/ms 12ms (cache hit)\n", NPM},
		{"grep", "a.go:12:func x()\nb.go:3:var y\nc.go:9:  z\n", Grep},
		{"ls", "total 8\ndrwxr-xr-x 2 u g 4096 Oct 1 12:00 .\n-rw-r--r-- 1 u g 12 Oct 1 12:00 a.txt\n", LS},
		{"log", "2026-10-01 12:00:00 INFO start\n2026-10-01 12:00:01 ERROR boom\n", Log},
		{"go code", "package main\n\nfunc main() {}\n", Code},
		{"python", "import os\n\ndef main():\n    pass\n", Code},
		{"ansi go test", "\x1b[32m=== RUN   TestX\x1b[0m\n", GoTest},
		{"prose", "Hello there.\nJust some words.\n", Text},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sniff(tt.text); got != tt.want {
				t.Fatalf("Sniff = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectPrecedence(t *testing.T) {
	// cmd wins over content; content wins when cmd unknown.
	if got := Detect(`{"a":1}`, Hints{Cmd: "git log"}); got != GitLog {
		t.Errorf("cmd precedence = %q", got)
	}
	if got := Detect(`{"a":1}`, Hints{Cmd: "curl https://x"}); got != JSON {
		t.Errorf("sniff fallback = %q", got)
	}
	if got := Detect("<p>x</p>", Hints{MIME: "text/html"}); got != HTML {
		t.Errorf("mime = %q", got)
	}
}

func TestLang(t *testing.T) {
	tests := []struct {
		text string
		h    Hints
		want string
	}{
		{"", Hints{File: "x.ts"}, LangCLike},
		{"", Hints{Cmd: "cat src/main.go"}, LangGo},
		{"package foo\n", Hints{}, LangGo},
		{"#!/bin/bash\necho\n", Hints{}, LangShell},
		{"import x from 'y'\n", Hints{}, LangCLike},
		{"plain words\n", Hints{}, ""},
	}
	for _, tt := range tests {
		if got := Lang(tt.text, tt.h); got != tt.want {
			t.Errorf("Lang(%q, %+v) = %q, want %q", tt.text, tt.h, got, tt.want)
		}
	}
}

func TestRefine(t *testing.T) {
	manifest := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: x\n"
	for _, c := range []struct{ text, cmd, file, want string }{
		{manifest, "cat pod.yaml", "", K8sManifest},
		{manifest, "", "pod.yml", K8sManifest},
		{"name: x\nitems: [1, 2]\n", "", "values.yaml", YAML},
		{`{"Reservations": [], "OwnerId": "1"}`, "", "out.json", AWS},
		{`{"a": 1}`, "", "out.json", JSON},
	} {
		if got := Detect(c.text, Hints{Cmd: c.cmd, File: c.file}); got != c.want {
			t.Errorf("%q %q: got %s, want %s", c.cmd, c.file, got, c.want)
		}
	}
}
