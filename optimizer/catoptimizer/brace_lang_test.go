package catoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
)

// Brace outline on syntax the generic c-like scan used to get wrong: each
// source must have its function bodies elided and its containers kept.
func TestBraceOutlineLanguages(t *testing.T) {
	tests := []struct {
		name, file, src string
		keep            []string // lines that must survive
	}{
		{"rust static lifetime", "a.rs", `fn f(x: &'static str) -> usize {
    first_body_line();
    second_body_line();
}

fn g<'a>(x: &'a str, y: &'a str) -> &'a str {
    third_body_line();
    fourth_body_line();
}
`, []string{"fn f(x: &'static str) -> usize {", "fn g<'a>(x: &'a str, y: &'a str) -> &'a str {"}},
		{"rust char and raw string", "a.rs", `fn f() -> char {
    let open = '{';
    let quote = '\'';
    let raw = r#"{ " {"#;
    first_body_line();
    open
}

fn g() {
    third_body_line();
    fourth_body_line();
}
`, []string{"fn f() -> char {", "fn g() {"}},
		{"rust mod and impl", "a.rs", `mod inner {
    pub struct S;
    impl S {
        pub fn run(&self) {
            first_body_line();
            second_body_line();
        }
    }
}
`, []string{"mod inner {", "impl S {", "pub fn run(&self) {"}},
		{"rust nested comment", "a.rs", `/* outer /* inner { */ still { comment */
fn f() {
    first_body_line();
    second_body_line();
}
`, []string{"fn f() {"}},
		{"d nested comment", "a.d", `/+ outer /+ inner { +/ still { +/
void f() {
    first_body_line();
    second_body_line();
}
`, []string{"void f() {"}},
		{"d union and template", "a.d", `union U {
    int a;
    void f() {
        first_body_line();
        second_body_line();
    }
}
`, []string{"union U {", "void f() {"}},
		{"cpp raw string", "a.cpp", `const char* s = R"x(" { )x";
void f() {
    first_body_line();
    second_body_line();
}
`, []string{"void f() {"}},
		{"cpp digit separator", "a.cpp", `const int big = 1'000'000;
void f() {
    first_body_line();
    second_body_line();
}
`, []string{"void f() {"}},
		{"cpp extern C", "a.cpp", `extern "C" {
void f() {
    first_body_line();
    second_body_line();
}
}
`, []string{`extern "C" {`, "void f() {"}},
		{"cpp template class", "a.hpp", `template <typename T>
class Box {
public:
    T get() const {
        first_body_line();
        second_body_line();
    }
};
`, []string{"class Box {", "T get() const {"}},
		{"scala package block", "a.scala", `package x {
  object O {
    def f(a: Int): Int = {
      first_body_line()
      second_body_line()
    }
  }
}
`, []string{"package x {", "object O {", "def f(a: Int): Int = {"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := opt(Outline).rewrite(tt.src, detect.LangCLike, detect.Hints{File: tt.file}, Outline)
			if strings.Contains(got, "_body_line") {
				t.Errorf("body kept:\n%s", got)
			}
			for _, want := range tt.keep {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in:\n%s", want, got)
				}
			}
		})
	}
}
