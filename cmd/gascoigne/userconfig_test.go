package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/configs"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/internal/registry"
)

// uncomment turns every "# setting" line of the template into a setting;
// "## docs" lines stay comments.
func uncomment(template []byte) []byte {
	lines := strings.Split(string(template), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "# ") {
			lines[i] = l[2:]
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func components(c *config.Config, k component.Kind) config.Components {
	return map[component.Kind]config.Components{
		component.KindExtension: c.Extensions, component.KindReceiver: c.Receivers,
		component.KindFilter: c.Filters, component.KindOptimizer: c.Optimizers, component.KindExporter: c.Exporters,
	}[k]
}

// decoded is every component's config as its factory sees it.
func decoded(t *testing.T, c *config.Config) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, f := range registry.Factories() {
		n, ok := components(c, f.Kind())[f.Type()]
		if !ok {
			continue
		}
		cfg := f.DefaultConfig()
		if err := config.DecodeNode(n, cfg); err != nil {
			t.Fatalf("%s %s: %v", f.Kind(), f.Type(), err)
		}
		out[string(f.Kind())+"/"+f.Type()] = cfg
	}
	return out
}

func TestUserTemplateIsTheDefaults(t *testing.T) {
	commented, err := overlay(configs.Default, configs.User)
	if err != nil {
		t.Fatal(err)
	}
	full, err := overlay(configs.Default, uncomment(configs.User))
	if err != nil {
		t.Fatal(err)
	}
	want, err := config.Parse(configs.Default)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"as written": commented, "every setting uncommented": full} {
		t.Run(name, func(t *testing.T) {
			got, err := config.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			g, w := decoded(t, got), decoded(t, want)
			for id := range w {
				if a, b := yamlOf(g[id]), yamlOf(w[id]); a != b {
					t.Errorf("%s differs from the default:\n%s\nwant:\n%s", id, a, b)
				}
			}
			if a, b := yamlOf(got.Router), yamlOf(want.Router); a != b {
				t.Errorf("router differs:\n%s\nwant:\n%s", a, b)
			}
			if a, b := yamlOf(got.Pipelines), yamlOf(want.Pipelines); a != b {
				t.Errorf("pipelines differ:\n%s\nwant:\n%s", a, b)
			}
		})
	}
}

// yamlOf compares configs by what they mean: nil and empty lists are equal.
func yamlOf(v any) string {
	out, _ := yaml.Marshal(v)
	return string(out)
}

// TestUserTemplateListsEverySetting keeps the template complete: a new
// config field fails here until it is documented in configs/user.yaml.
func TestUserTemplateListsEverySetting(t *testing.T) {
	var tmpl map[string]any
	if err := yaml.Unmarshal(uncomment(configs.User), &tmpl); err != nil {
		t.Fatal(err)
	}
	def, err := config.Parse(configs.Default)
	if err != nil {
		t.Fatal(err)
	}
	sections := map[component.Kind]string{
		component.KindExtension: "extensions", component.KindReceiver: "receivers",
		component.KindFilter: "filters", component.KindOptimizer: "optimizers", component.KindExporter: "exporters",
	}
	for _, f := range registry.Factories() {
		if _, used := components(def, f.Kind())[f.Type()]; !used {
			continue
		}
		section, _ := tmpl[sections[f.Kind()]].(map[string]any)
		got, ok := section[f.Type()]
		if !ok {
			t.Errorf("%s.%s missing", sections[f.Kind()], f.Type())
			continue
		}
		missingFields(t, sections[f.Kind()]+"."+f.Type(), reflect.TypeOf(f.DefaultConfig()).Elem(), got)
	}
}

func missingFields(t *testing.T, path string, typ reflect.Type, got any) {
	t.Helper()
	m, _ := got.(map[string]any)
	for i := range typ.NumField() {
		f := typ.Field(i)
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		v, ok := m[name]
		if !ok {
			t.Errorf("%s.%s missing", path, name)
			continue
		}
		if f.Type.Kind() == reflect.Struct && f.Type != reflect.TypeOf(time.Duration(0)) {
			missingFields(t, path+"."+name, f.Type, v)
		}
	}
}

func TestOverlay(t *testing.T) {
	base := "a: {x: 1, y: 2}\nlist: [1, 2]\nkeep: k\n"
	tests := []struct{ name, over, want string }{
		{"comments only", "## nothing set\n# a: {x: 9}\n", base},
		{"empty", "", base},
		{"maps merge", "a: {x: 9}\n", "a: {x: 9, y: 2}\nlist: [1, 2]\nkeep: k\n"},
		{"lists replace", "list: [3]\n", "a: {x: 1, y: 2}\nlist: [3]\nkeep: k\n"},
		{"new key", "b: 5\n", base + "b: 5\n"},
		{"parent uncommented alone", "a:\n  # x: 9\n", base},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := overlay([]byte(base), []byte(tt.over))
			if err != nil {
				t.Fatal(err)
			}
			var g, w any
			yaml.Unmarshal(got, &g)
			yaml.Unmarshal([]byte(tt.want), &w)
			if !reflect.DeepEqual(g, w) {
				t.Fatalf("got:\n%s", got)
			}
		})
	}
	if _, err := overlay([]byte(base), []byte("a: [unclosed\n")); err == nil {
		t.Fatal("bad YAML accepted")
	}
}

func TestFirstRunWritesUserConfig(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_CONFIG", "")
	dir, _ := os.UserConfigDir()
	path := filepath.Join(dir, "gascoigne", "config.yaml")

	data, err := configData("")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(configs.User) {
		t.Fatal("template not written on first run")
	}
	if _, err := config.Parse(data); err != nil {
		t.Fatalf("first-run config does not load: %v", err)
	}

	if err := os.WriteFile(path, []byte("optimizers:\n  grep: { max_per_file: 50 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data, err = configData("")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !strings.Contains(string(got), "max_per_file: 50") {
		t.Fatal("an existing config was overwritten")
	}
	c, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	cfgs := decoded(t, c)
	if g := reflect.ValueOf(cfgs["optimizer/grep"]).Elem().FieldByName("MaxPerFile").Int(); g != 50 {
		t.Fatalf("user setting not applied: %d", g)
	}
	if _, ok := cfgs["optimizer/doc"]; !ok || len(c.Pipelines) == 0 {
		t.Fatal("defaults lost under a partial user config")
	}
}

func TestExplicitConfigSkipsFirstRun(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	own := filepath.Join(t.TempDir(), "own.yaml")
	if err := os.WriteFile(own, []byte("filters:\n  minsize: { min_tokens: 10 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GASCOIGNE_CONFIG", own)
	if _, err := configData(""); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(home, "gascoigne", "config.yaml")) {
		t.Fatal("wrote a user config although GASCOIGNE_CONFIG names one")
	}
}

// TestDefaultsListEverySetting keeps configs/default.yaml explicit: every
// field of every used component is written out, so the effective config
// is readable without the Go sources.
func TestDefaultsListEverySetting(t *testing.T) {
	var raw map[string]any
	if err := yaml.Unmarshal(configs.Default, &raw); err != nil {
		t.Fatal(err)
	}
	def, err := config.Parse(configs.Default)
	if err != nil {
		t.Fatal(err)
	}
	sections := map[component.Kind]string{
		component.KindExtension: "extensions", component.KindReceiver: "receivers",
		component.KindFilter: "filters", component.KindOptimizer: "optimizers", component.KindExporter: "exporters",
	}
	for _, f := range registry.Factories() {
		if _, used := components(def, f.Kind())[f.Type()]; !used {
			continue
		}
		section, _ := raw[sections[f.Kind()]].(map[string]any)
		missingFields(t, sections[f.Kind()]+"."+f.Type(), reflect.TypeOf(f.DefaultConfig()).Elem(), section[f.Type()])
	}
}
