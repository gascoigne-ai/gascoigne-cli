package trace

import (
	"cmp"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// runtime describes the stack frames of one language runtime: which lines
// are frames, which of them belong to the runtime or a framework (and tell
// an agent nothing about its own code), and the detail line some runtimes
// print under every frame.
type runtime struct {
	frame  *lazyre.Regexp // a frame line
	noise  *lazyre.Regexp // a frame inside the runtime or a framework
	detail *lazyre.Regexp // optional line under a frame that goes with it
	what   string         // marker wording, "framework stack frames" when empty
	// keepTop keeps the first frame under a header line: where the exception
	// was thrown, even when that is inside the runtime (Objects.requireNonNull).
	keepTop bool
}

func re(s string) *lazyre.Regexp { return lazyre.New(s) }

var runtimes = []runtime{
	{ // JVM: java, scala, kotlin
		keepTop: true,
		frame:   re(`^\s+at [\w$.<>/]+\(.*\)$`),
		noise:   re(`^\s+at (java\.|javax\.|jdk\.|sun\.|com\.sun\.|scala\.|kotlin\.|kotlinx\.|org\.junit|org\.scalatest|org\.apache\.(maven|catalina|tomcat)|org\.springframework\.(aop|cglib|web\.servlet|web\.filter)|io\.netty|akka\.|cats\.effect|zio\.|reactor\.core)`),
	},
	{ // Python: a File line and the source line under it
		frame:  re(`^\s+File ".*", line \d+`),
		noise:  re(`^\s+File ".*(site-packages|dist-packages|/lib/python3?[\d.]*/)`),
		detail: re(`^\s{4}\S`),
	},
	{ // Rust: "  N: path::fn" and "  at file:line" under it
		frame:  re(`^\s+\d+: \S`),
		noise:  re(`^\s+\d+: (std::|core::|alloc::|tokio::|futures_|rust_begin_unwind|__rust|<(alloc|core|std)::|<tokio::|_start|__libc|main$)`),
		detail: re(`^\s+at \S`),
	},
	{ // Ruby
		frame: re(`^\s+(from )?\S+:\d+:in [` + "`" + `'<]`),
		noise: re(`^\s+(from )?\S*(/gems/|/rubygems/|/ruby/\d|<internal:)`),
	},
	{ // .NET
		keepTop: true,
		frame:   re(`^\s+at [\w.` + "`" + `<>+\[\],]+\(.*\)( in .+:line \d+)?$`),
		noise:   re(`^\s+at (System\.|Microsoft\.|Xunit\.|NUnit\.)`),
	},
	{ // PHP
		frame: re(`^#\d+ \S+\(\d+\): |^#\d+ \[internal function\]`),
		noise: re(`^#\d+ \S*vendor/|^#\d+ \[internal function\]`),
	},
	{ // gdb, lldb, sanitizers, valgrind: "#3 0x... in fn file:line", "#1 0x...  (/usr/lib/libc.so.6+0x27780)"
		frame: re(`^\s*#\d+ +0x[0-9a-f]+ |^\s*==\d+==\s+(at|by) 0x`),
		noise: re(`^\s*#\d+ +0x[0-9a-f]+ in (__libc|_start|start_thread|clone|std::|__gnu_cxx|__cxx11|__asan|__interceptor|__sanitizer|operator new|operator delete)|^\s*#\d+ +0x[0-9a-f]+ +(in \S+ )?\(/(usr/)?lib[^)]*\.so[\w.]*\+0x[0-9a-f]+\)|^\s*==\d+==\s+(at|by) 0x[0-9A-F]+: (\(below main\)|__libc|_start|start_thread|operator new|malloc|calloc|realloc)`),
	},
	{ // AddressSanitizer's shadow memory dump and legend
		frame: re(`^Shadow bytes around the buggy address:$|^\s*(=>)?0x[0-9a-f]+:( |\[)[0-9a-f]{2}|^Shadow byte legend|^  [A-Z][A-Za-z ]+:\s+[0-9a-f]{2}( [0-9a-f]{2})*\s*$`),
		noise: re(`^Shadow bytes around the buggy address:$|^\s*(=>)?0x[0-9a-f]+:( |\[)[0-9a-f]{2}|^Shadow byte legend|^  [A-Z][A-Za-z ]+:\s+[0-9a-f]{2}( [0-9a-f]{2})*\s*$`),
		what:  "sanitizer shadow lines",
	},
	{ // D
		frame: re(`^\?\?:\? |^\S+\.d:\d+ `),
		noise: re(`^\?\?:\? `),
	},
}

// Frames drops runs of at least minRun frames that lie inside a language
// runtime or framework (JVM, Python, Rust, Ruby, .NET, PHP, native
// debuggers, D), one marker per run. Frames of the program stay, so does
// every frame that is not part of such a run.
func Frames(lines []string, minRun int) []string {
	var out []string
	for i := 0; i < len(lines); {
		rt, n := noiseRun(lines, i)
		if rt == nil {
			out = append(out, lines[i])
			i++
			continue
		}
		if rt.keepTop && (i == 0 || !rt.frame.MatchString(lines[i-1])) {
			out = append(out, lines[i])
			i++
			n--
		}
		frames := countFrames(rt, lines[i:i+n])
		if frames < minRun {
			out = append(out, lines[i:i+n]...)
		} else {
			out = append(out, compress.Omitted(frames, cmp.Or(rt.what, "framework stack frames")))
		}
		i += n
	}
	return out
}

// noiseRun finds the runtime whose noise frames start at lines[i] and how
// many lines the run covers, details included. nil when i starts none.
func noiseRun(lines []string, i int) (*runtime, int) {
	for k := range runtimes {
		rt := &runtimes[k]
		if !rt.noise.MatchString(lines[i]) {
			continue
		}
		n := 0
		for i+n < len(lines) && rt.noise.MatchString(lines[i+n]) {
			n++
			for rt.detail != nil && i+n < len(lines) && !rt.frame.MatchString(lines[i+n]) && rt.detail.MatchString(lines[i+n]) {
				n++
			}
		}
		return rt, n
	}
	return nil, 0
}

func countFrames(rt *runtime, lines []string) int {
	n := 0
	for _, l := range lines {
		if rt.frame.MatchString(l) {
			n++
		}
	}
	return n
}
