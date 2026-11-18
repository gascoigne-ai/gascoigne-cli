package taskoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

func TestGradleNoiseRulesVerify(t *testing.T) {
	if err := gradleNoise.Verify(); err != nil {
		t.Fatal(err)
	}
}

// The regexp gradleNoise replaced, kept to pin the old behavior.
var oldGradleNoise = lazyre.New(`^(Downloading https?://|Download https?://|Starting a Gradle Daemon|<[-=]+> \d+% |Deprecated Gradle features were used|You can use '--warning-mode all'|For more on this, please refer to|\* Get more help at|> Run with --(stacktrace|info|debug|scan)|> Get more help|Configuration cache entry)`)

func TestGradleNoiseSameAsRegexp(t *testing.T) {
	var ex []string
	for _, r := range gradleNoise {
		ex = append(ex, r.Yes...)
		ex = append(ex, r.No...)
	}
	ex = append(ex, "Downloading https://services.gradle.org/distributions/gradle-8.5-bin.zip", "Downloading http://x", "Downloading ftp://x", "Download https://repo.maven.org/x.pom", "Download http://x",
		"Starting a Gradle Daemon (subsequent builds will be faster)", "Starting a Gradle Daemon, 1 busy", "<-------------> 0% INITIALIZING [75ms]", "<===--------> 30% EXECUTING [1s]", "<=> 5% x", "<> 5% x", "<=-> 5%x", "<=> % x",
		"Deprecated Gradle features were used in this build, making it incompatible with Gradle 9.0.", "You can use '--warning-mode all' to show the individual deprecation warnings", "For more on this, please refer to https://docs.gradle.org/",
		"* Get more help at https://help.gradle.org", "> Run with --stacktrace option to get the stack trace.", "> Run with --info option", "> Run with --debug option", "> Run with --scan to get full insights.", "> Run with --other", "> Get more help", "Configuration cache entry stored.", "Configuration cache entry reused.")
	trimmed := func(line string) bool { return oldGradleNoise.MatchString(strings.TrimSpace(line)) }
	now := func(line string) bool { return gradleNoise.Match(strings.TrimSpace(line)) }
	sameAsOld(t, trimmed, now, ex)
	sameAsOld(t, oldGradleNoise.MatchString, gradleNoise.Match, ex)
}
