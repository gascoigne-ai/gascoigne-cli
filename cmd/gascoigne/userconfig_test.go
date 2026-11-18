package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

func components(settings *config.Config, kind component.Kind) config.Components {
	return map[component.Kind]config.Components{
		component.KindExtension: settings.Extensions, component.KindReceiver: settings.Receivers,
		component.KindFilter: settings.Filters, component.KindOptimizer: settings.Optimizers, component.KindExporter: settings.Exporters,
	}[kind]
}

// decoded is every component's config as its factory sees it.
func decoded(t *testing.T, settings *config.Config) map[string]any {
	t.Helper()
	out := map[string]any{}
	for _, f := range registry.Factories() {
		n, ok := components(settings, f.Kind())[f.Type()]
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
func yamlOf(value any) string {
	out, _ := yaml.Marshal(value)
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
	for f := range typ.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		v, ok := m[name]
		if !ok {
			t.Errorf("%s.%s missing", path, name)
			continue
		}
		if f.Type.Kind() == reflect.Struct && f.Type != reflect.TypeFor[time.Duration]() {
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

// TestTemplatesCarryVersion: the embedded files state the format they are in,
// and the key configs skips as a non-setting is the one Config decodes.
func TestTemplatesCarryVersion(t *testing.T) {
	for name, data := range map[string][]byte{"default": configs.Default, "user": configs.User} {
		t.Run(name, func(t *testing.T) {
			if !slices.ContainsFunc(strings.Split(string(data), "\n"), func(line string) bool { return strings.HasPrefix(line, "version: 1") }) {
				t.Fatal("no uncommented `version: 1` line")
			}
			got, err := config.CheckVersion(data)
			if err != nil || got != config.SupportedVersion {
				t.Fatalf("CheckVersion = %d, %v, want %d", got, err, config.SupportedVersion)
			}
		})
	}
	field, _ := reflect.TypeFor[config.Config]().FieldByName("Version")
	if tag, _, _ := strings.Cut(field.Tag.Get("yaml"), ","); tag != configs.VersionKey {
		t.Errorf("Config.Version yaml tag %q != configs.VersionKey %q", tag, configs.VersionKey)
	}
	cfg, err := config.Parse(configs.Default)
	if err != nil || cfg.Version != config.SupportedVersion {
		t.Errorf("default Version = %v, %v", cfg, err)
	}
}

func TestConfigFileVersion(t *testing.T) {
	tests := []struct {
		name, data string
		err        string
	}{
		{"missing loads", "filters:\n  minsize: { min_tokens: 10 }\n", ""},
		{"1 loads", "version: 1\nfilters:\n  minsize: { min_tokens: 10 }\n", ""},
		{"2 refused", "version: 2\n", "version 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sandbox(t)
			own := filepath.Join(t.TempDir(), "own.yaml")
			write(t, own, tt.data)
			t.Setenv("GASCOIGNE_CONFIG", own)
			data, err := configData("")
			if tt.err == "" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := config.Parse(data); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("a newer config version loaded")
			}
			for _, want := range []string{own, tt.err, "supports (1)"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestProjectConfigVersion(t *testing.T) {
	sandbox(t)
	repo := t.TempDir()
	t.Chdir(repo)
	path := filepath.Join(repo, projectConfigName)
	write(t, path, "version: 2\n")
	base, err := configFile("")
	if err != nil {
		t.Fatal(err)
	}
	_, err = withProject(base)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "version 2") {
		t.Errorf("err = %v, want it to name %s and version 2", err, path)
	}
}
