package detect

// Logs.
const Log = "log/app"

func init() {
	register(
		Spec{Type: Log, Files: []string{"*.log"}, Priority: 80,
			Sniff: ratio(`^\[?(\d{4}-\d\d-\d\d[T ]\d\d:\d\d|\d\d:\d\d:\d\d|[A-Z][a-z]{2} [ \d]\d \d\d:\d\d)|^\[?(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\b|\blevel=(debug|info|warn|error)`, 0.5)},
	)
}
