package taskoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
)

var (
	gradleTask  = lazyre.New(`^> Task (:\S+)(?: (UP-TO-DATE|NO-SOURCE|FROM-CACHE|SKIPPED|FAILED))?$`)
	gradleNoise = pcomb.Rules{
		{
			Name:  "download",
			Match: pcomb.Prefix("Downloading http://", "Downloading https://", "Download http://", "Download https://"),
			Yes:   []string{"Downloading https://services.gradle.org/distributions/gradle-8.5-bin.zip", "Download https://repo.maven.apache.org/maven2/x.pom"},
			No:    []string{"Downloading ftp://x", "Downloaded https://x"},
		},
		{
			Name:  "daemon",
			Match: pcomb.Prefix("Starting a Gradle Daemon"),
			Yes:   []string{"Starting a Gradle Daemon (subsequent builds will be faster)"},
			No:    []string{"Starting Gradle"},
		},
		{
			Name:  "progress bar",
			Match: pcomb.Re(`^<[-=]+> \d+% `),
			Yes:   []string{"<-------------> 0% INITIALIZING [75ms]", "<===--------> 30% EXECUTING [1s]"},
			No:    []string{"<> 5% x", "<=> 5%x"},
		},
		{
			Name: "build footer",
			Match: pcomb.Prefix("Deprecated Gradle features were used", "You can use '--warning-mode all'", "For more on this, please refer to", "* Get more help at",
				"> Run with --stacktrace", "> Run with --info", "> Run with --debug", "> Run with --scan", "> Get more help", "Configuration cache entry"),
			Yes: []string{"Deprecated Gradle features were used in this build, making it incompatible with Gradle 9.0.", "You can use '--warning-mode all' to show the individual deprecation warnings",
				"* Get more help at https://help.gradle.org", "> Run with --stacktrace option to get the stack trace.", "> Run with --scan to get full insights.", "Configuration cache entry stored."},
			No: []string{"> Run with --other", "> Task :app:build"},
		},
	}
)

// gradle: "> Task :x" sections; quiet up-to-date/cached tasks are counted.
func gradle(lines []string) []string {
	var head, tail []string
	var sections []section
	status := map[string]int{}
	var cur *section
	for _, l := range lines {
		switch m := gradleTask.FindStringSubmatch(l); {
		case gradleNoise.Match(strings.TrimSpace(l)):
		case m != nil:
			state := m[2]
			if state == "" {
				state = "executed"
			}
			status[state]++
			sections = append(sections, section{name: m[1], failed: state == "FAILED", cached: state != "executed" && state != "FAILED"})
			cur = &sections[len(sections)-1]
		case strings.HasPrefix(l, "BUILD ") || strings.HasPrefix(l, "FAILURE:") || strings.HasPrefix(l, "* What went wrong") || strings.HasPrefix(l, "* Try:") || strings.HasPrefix(l, "* Exception"):
			cur = nil
			tail = append(tail, l)
		case cur != nil:
			cur.body = append(cur.body, l)
		case len(sections) == 0:
			head = append(head, l)
		default:
			tail = append(tail, l)
		}
	}
	// Executed tasks with output (warnings) are worth showing; silent ones are not.
	var shown []section
	for _, s := range sections {
		if s.failed || hasFailure(s.body) {
			s.failed = true
			shown = append(shown, s)
		}
	}
	var counts []string
	for _, st := range []string{"executed", "UP-TO-DATE", "FROM-CACHE", "NO-SOURCE", "SKIPPED", "FAILED"} {
		if n := status[st]; n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", n, strings.ToLower(st)))
		}
	}
	if len(counts) > 0 {
		head = append(head, "tasks: "+strings.Join(counts, ", "))
	}
	return render(head, shown, tail)
}
