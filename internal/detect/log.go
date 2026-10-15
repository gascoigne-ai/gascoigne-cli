package detect

// Logs.
const (
	Log        = "log/app"
	Journal    = "log/journal" // journalctl
	Syslog     = "log/syslog"
	DockerLogs = "log/docker"  // docker/podman/compose logs
	KubeLogs   = "log/kubectl" // kubectl logs, stern
	Access     = "log/access"  // nginx/apache combined access logs
	CloudWatch = "log/cloudwatch"
	GCPLogs    = "log/gcp" // gcloud logging read
	AzureLogs  = "log/azure"
)

// syslogLine is "Oct 10 07:00:01 host unit[pid]: message".
const syslogLine = `^[A-Z][a-z]{2} [ \d]\d \d\d:\d\d:\d\d \S+ [\w@./-]+(\[\d+\])?: `

func init() {
	register(
		Spec{Type: Journal, Cmds: []string{"journalctl"}, Priority: 60,
			Sniff: all(firstLine(`^-- (Logs begin at|Journal begins at|Boot [0-9a-f]+ --)`), ratio(syslogLine+`|^-- `, 0.6))},
		Spec{Type: Syslog, Files: []string{"syslog", "messages", "auth.log", "kern.log", "daemon.log", "syslog.*", "messages.*"}, Priority: 61,
			Sniff: ratio(syslogLine, 0.6)},
		// Prefixed by the source: compose "web-1  | …", kubectl --prefix "[pod/web-x/app] …".
		Spec{Type: DockerLogs, Cmds: []string{"docker logs", "docker compose logs", "docker-compose logs", "podman logs", "docker service logs"}, Priority: 63,
			Sniff: all(ratio(`^[A-Za-z0-9][\w.-]*\s+\| `, 0.8), noLine(`^-+\+-`))}, // psql rules
		Spec{Type: KubeLogs, Cmds: []string{"kubectl logs", "oc logs", "k logs", "stern"}, Priority: 63,
			Sniff: ratio(`^\[(pod|deployment|statefulset|daemonset|job|replicaset)/[^\]\s]+\] `, 0.8)},
		Spec{Type: Access, Files: []string{"access.log", "access_log", "*access.log", "*access.log.*", "*-access_log"}, Priority: 62,
			Sniff: ratio(accessLine, 0.6)},
		Spec{Type: CloudWatch, Cmds: []string{"aws logs tail", "aws logs filter-log-events", "aws logs get-log-events", "saw get", "saw watch"}},
		Spec{Type: GCPLogs, Cmds: []string{"gcloud logging read", "gcloud logging tail", "gcloud beta logging tail", "gcloud app logs read", "gcloud functions logs read"}},
		Spec{Type: AzureLogs, Cmds: []string{"az monitor activity-log list", "az monitor log-analytics query", "az webapp log tail", "az containerapp logs show"}},
		Spec{Type: Log, Files: []string{"*.log"}, Priority: 80,
			Sniff: ratio(`^\[?(\d{4}-\d\d-\d\d[T ]\d\d:\d\d|\d\d:\d\d:\d\d|[A-Z][a-z]{2} [ \d]\d \d\d:\d\d)|^\[?(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL)\b|\blevel=(debug|info|warn|error)`, 0.5)},
	)
}

// accessLine is the common/combined log format: ip - user [time] "GET /x HTTP/1.1" 200 123.
const accessLine = `^\S+ \S+ \S+ \[[^\]]+\] "[A-Z]+ \S+ [^"]*" \d{3} (\d+|-)`
